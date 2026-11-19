// Avisos de "el repositorio pudo cambiar en disco" para el panel lateral.
//
// Los eventos de Wails (workspace:changed / pty:exit) cubren parte de los
// casos, pero no llegan cuando el cambio lo hace el propio frontend (un
// commit desde el panel) ni cuando se edita el repo en otra ventana. Este bus
// local reúne todas esas señales en un solo sitio al que se suscribe el
// explorador, sin acoplarlo al App ni al store de commits.

type Listener = () => void

const listeners = new Set<Listener>()

export function onRepoChanged(cb: Listener): () => void {
  listeners.add(cb)
  return () => {
    listeners.delete(cb)
  }
}

export function notifyRepoChanged() {
  for (const cb of listeners) cb()
}
