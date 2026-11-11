//go:build windows

package lock

import "testing"

// TestTryAcquireOneInstancePerProject comprueba que un mismo proyecto no admite
// dos candados y que cerrar el primero lo libera.
func TestTryAcquireOneInstancePerProject(t *testing.T) {
	key := t.TempDir()

	first, free, err := TryAcquire(key)
	if err != nil {
		t.Fatalf("primer candado: %v", err)
	}
	if !free {
		t.Fatal("el primer candado deberia obtenerse")
	}

	second, free, err := TryAcquire(key)
	if err != nil {
		t.Fatalf("segundo candado: %v", err)
	}
	if free {
		second.Close()
		t.Fatal("no deberia haber dos instancias para el mismo proyecto")
	}

	if err := first.Close(); err != nil {
		t.Fatalf("cerrar el primer candado: %v", err)
	}

	third, free, err := TryAcquire(key)
	if err != nil {
		t.Fatalf("tercer candado: %v", err)
	}
	if !free {
		t.Fatal("el candado no se libero al cerrarse")
	}
	third.Close()
}

// TestTryAcquireDifferentProjects comprueba que dos proyectos distintos no
// comparten candado.
func TestTryAcquireDifferentProjects(t *testing.T) {
	a, free, err := TryAcquire(t.TempDir())
	if err != nil || !free {
		t.Fatalf("candado A: free=%v err=%v", free, err)
	}
	defer a.Close()

	b, free, err := TryAcquire(t.TempDir())
	if err != nil {
		t.Fatalf("candado B: %v", err)
	}
	if !free {
		t.Fatal("dos proyectos distintos deberian poder abrirse a la vez")
	}
	b.Close()
}

// TestLockPathIsStable comprueba que dos formas de escribir la misma ruta dan el
// mismo fichero de candado.
func TestLockPathIsStable(t *testing.T) {
	key := t.TempDir()
	a, err := lockPath(key)
	if err != nil {
		t.Fatal(err)
	}
	b, err := lockPath(key + `\`)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("rutas distintas para el mismo proyecto:\n%s\n%s", a, b)
	}
}
