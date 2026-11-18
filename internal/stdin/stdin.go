// Package stdin centraliza la lectura de la entrada estandar.
//
// Antes cada prompt creaba su propio bufio.Reader sobre os.Stdin; como el
// reader bufferea de mas, la primera lectura se tragaba las lineas siguientes
// (pegar varias respuestas o alimentar la tarea por una tuberia). Un unico
// reader compartido mantiene el orden de las lineas.
//
// El reader NO puede crearse en init: AttachParentConsole sustituye os.Stdin
// por el handle de la consola padre despues de arrancar, y un reader creado
// antes leeria de un handle invalido (EOF inmediato). Por eso se crea (y se
// recrea si os.Stdin cambia) en la primera lectura.
package stdin

import (
	"bufio"
	"os"
	"strings"
	"sync"
)

var (
	mu      sync.Mutex
	reader  *bufio.Reader
	current *os.File
)

func get() *bufio.Reader {
	if reader == nil || current != os.Stdin {
		current = os.Stdin
		reader = bufio.NewReader(os.Stdin)
	}
	return reader
}

// ReadLine devuelve la siguiente linea sin CR/LF ("" si no hay entrada).
func ReadLine() string {
	line, _ := ReadLineErr()
	return line
}

// ReadLineErr es ReadLine pero propagando el error (EOF incluido).
func ReadLineErr() (string, error) {
	mu.Lock()
	defer mu.Unlock()
	line, err := get().ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	// Una linea sin '\n' final (EOF con contenido) sigue siendo valida.
	return strings.TrimRight(line, "\r\n"), nil
}
