// Notificaciones globales: el host vive en App.svelte (nunca se desmonta),
// así que el aviso de una tarea sobrevive a cambios de pestaña y de proyecto.
export type ToastKind = 'success' | 'error' | 'info' | 'warn'

export interface Toast {
  id: number
  kind: ToastKind
  title: string
  detail: string
}

const toasts = $state<Toast[]>([])
const timers = new Map<number, ReturnType<typeof setTimeout>>()
let nextId = 1

// Lectura pura para usar desde un $derived en el componente.
export function toastList(): Toast[] {
  return toasts
}

export function dismissToast(id: number) {
  const timer = timers.get(id)
  if (timer) clearTimeout(timer)
  timers.delete(id)
  const i = toasts.findIndex((t) => t.id === id)
  if (i >= 0) toasts.splice(i, 1)
}

export function pushToast(input: { kind?: ToastKind; title: string; detail?: string }): number {
  // Los mensajes de git pueden traer varias líneas: solo cabe el asunto.
  const detail = (input.detail ?? '').split(/\r?\n/)[0].trim().slice(0, 160)
  const toast: Toast = {
    id: nextId++,
    kind: input.kind ?? 'info',
    title: input.title,
    detail
  }
  toasts.push(toast)
  while (toasts.length > 4) dismissToast(toasts[0].id)
  timers.set(
    toast.id,
    setTimeout(() => dismissToast(toast.id), toast.kind === 'error' ? 7000 : 4500)
  )
  return toast.id
}
