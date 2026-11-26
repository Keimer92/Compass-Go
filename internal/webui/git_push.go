package webui

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"compass/internal/windialog"
)

// PushProgress es el avance de un push. Se emite por el evento
// "git:pushprogress" mientras `git push --progress` escribe en stderr, para que
// el asistente de vincular repositorio pueda mostrar una barra con los MB
// enviados y los que faltan.
type PushProgress struct {
	// Stage: counting | compressing | writing | resolving | done.
	Stage string `json:"stage"`
	// Percent es el avance global 0-100, ponderado por fase (no es el tanto por
	// ciento de la fase, que git reinicia en cada una).
	Percent  int   `json:"percent"`
	Uploaded int64 `json:"uploaded"` // bytes ya enviados
	Total    int64 `json:"total"`    // bytes a enviar; 0 si no se pudo calcular
	Speed    int64 `json:"speed"`    // bytes/s que informa git; 0 si no lo da
}

var (
	// "Writing objects:  45% (5/11)" con prefijo opcional "remote: ".
	pushLineRe = regexp.MustCompile(`(?i)(enumerating objects|counting objects|compressing objects|writing objects|resolving deltas):\s+(\d+)%`)
	pushSizeRe = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*(bytes|KiB|MiB|GiB)\b`)
	// El tamano con "/s" es la velocidad: se quita antes de buscar el enviado.
	pushSpeedRe = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)\s*(bytes|KiB|MiB|GiB)/s`)
)

// parsePushLine interpreta una linea de progreso de git. Devuelve false si la
// linea no lo es (avisos de remote:, "Everything up-to-date", errores...).
func parsePushLine(line string) (PushProgress, bool) {
	m := pushLineRe.FindStringSubmatchIndex(line)
	if m == nil {
		return PushProgress{}, false
	}
	name := strings.ToLower(line[m[2]:m[3]])
	pct, _ := strconv.Atoi(line[m[4]:m[5]])
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}

	p := PushProgress{}
	// Cada fase ocupa una franja del total: comprimir y escribir son las que
	// de verdad consumen tiempo en un push normal.
	switch {
	case strings.HasPrefix(name, "enumerating"), strings.HasPrefix(name, "counting"):
		p.Stage = "counting"
		p.Percent = pct * 10 / 100
	case strings.HasPrefix(name, "compressing"):
		p.Stage = "compressing"
		p.Percent = 10 + pct*20/100
	case strings.HasPrefix(name, "writing"):
		p.Stage = "writing"
		p.Percent = 30 + pct*65/100
	case strings.HasPrefix(name, "resolving"):
		p.Stage = "resolving"
		p.Percent = 95 + pct*5/100
	}

	tail := line[m[1]:]
	if sp := pushSpeedRe.FindStringSubmatch(tail); sp != nil {
		p.Speed = int64(sizeToBytes(sp[1], sp[2]))
		tail = strings.Replace(tail, sp[0], "", 1)
	}
	if sz := pushSizeRe.FindStringSubmatch(tail); sz != nil {
		p.Uploaded = int64(sizeToBytes(sz[1], sz[2]))
	}
	return p, true
}

// sizeToBytes convierte "1.23" + "MiB" a bytes.
func sizeToBytes(num, unit string) float64 {
	f, _ := strconv.ParseFloat(num, 64)
	switch strings.ToLower(unit) {
	case "kib":
		f *= 1024
	case "mib":
		f *= 1024 * 1024
	case "gib":
		f *= 1024 * 1024 * 1024
	}
	return f
}

// splitProgressLines parte por \r y \n: git separa las actualizaciones de una
// misma fase con retorno de carro cuando no hay terminal.
func splitProgressLines(data []byte, atEOF bool) (int, []byte, error) {
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF && len(data) > 0 {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// pushTransferSize estima en bytes lo que git enviara al remoto, sumando el
// tamano en disco (ya comprimido) de los objetos que faltan en el upstream.
// Es una estimacion: el pack final anade cabeceras y deltas, pero se ajusta lo
// suficiente para ensenar "X MB de Y MB". Devuelve 0 si no se puede calcular.
func pushTransferSize(dir, upstream string) int64 {
	rev := "HEAD"
	if upstream != "" {
		rev = upstream + "..HEAD"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	list := exec.CommandContext(ctx, "git", "rev-list", "--objects", "--no-object-names", rev)
	windialog.NoWindow(list)
	list.Dir = dir
	pipe, err := list.StdoutPipe()
	if err != nil {
		return 0
	}
	check := exec.CommandContext(ctx, "git", "cat-file", "--batch-check=%(objectsize:disk)")
	windialog.NoWindow(check)
	check.Dir = dir
	check.Stdin = pipe
	out, err := check.StdoutPipe()
	if err != nil {
		return 0
	}
	if err := list.Start(); err != nil {
		return 0
	}
	if err := check.Start(); err != nil {
		_ = list.Wait()
		return 0
	}

	var total int64
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if n, err := strconv.ParseInt(strings.TrimSpace(sc.Text()), 10, 64); err == nil && n > 0 {
			total += n
		}
	}
	_ = check.Wait()
	_ = list.Wait()
	return total
}

// runGitPushProgress ejecuta `git push --progress`, reenvia el avance al
// frontend y devuelve la salida completa (stdout+stderr) para clasificar
// errores igual que antes.
//
// git escribe el progreso en stderr y solo lo hace si se lo piden con
// --progress (sin TTY no lo activa solo). LC_ALL=C fija los textos en ingles:
// los mensajes de error y las fases se analizan por texto.
func runGitPushProgress(dir string, total int64, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", args...)
	windialog.NoWindow(cmd)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C", "GIT_PROGRESS_DELAY=0")

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var logs strings.Builder
	var uploaded int64
	sc := bufio.NewScanner(stderr)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	sc.Split(splitProgressLines)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		logs.WriteString(line)
		logs.WriteByte('\n')
		p, ok := parsePushLine(line)
		if !ok {
			continue
		}
		// El contador de bytes de git puede retroceder al cambiar de fase
		// (writing -> resolving): se queda con el maximo visto.
		if p.Uploaded > uploaded {
			uploaded = p.Uploaded
		}
		p.Uploaded = uploaded
		p.Total = total
		Emit("git:pushprogress", p)
	}

	waitErr := cmd.Wait()
	out := strings.TrimSpace(logs.String())
	if s := strings.TrimSpace(stdout.String()); s != "" {
		if out != "" {
			out += "\n"
		}
		out += s
	}
	if ctx.Err() != nil {
		return out, fmt.Errorf("git %s: tiempo de espera agotado", args[0])
	}
	if waitErr != nil {
		return out, waitErr
	}
	Emit("git:pushprogress", PushProgress{Stage: "done", Percent: 100, Uploaded: uploaded, Total: total})
	return out, nil
}
