import { Call, Events } from '@wailsio/runtime'

export interface ProjectInfo { path: string; name: string; type: string }
export interface MenuItemInfo {
  id: string
  label: string
  description: string
  icon: string
  color: string
  section: string
}
export interface WorkspaceInfo { projects: ProjectInfo[]; active: number }

const call = <T>(method: string, ...args: any[]): Promise<T> =>
  Call.ByName(method, ...args) as unknown as Promise<T>

const M = (name: string) => `compass/internal/webui.Bridge.${name}`

export const getWorkspace = () => call<WorkspaceInfo>(M('GetWorkspace'))
export const selectProject = (i: number) => call<WorkspaceInfo>(M('SelectProject'), i)
export const removeProject = (i: number) => call<WorkspaceInfo>(M('RemoveProject'), i)
export const getMenu = () => call<MenuItemInfo[]>(M('GetMenu'))
export const runTask = (id: string) => call<string>(M('RunTask'), id)
export interface Subcard { task: string; label: string; description: string; icon: string }
export const subcards = (id: string) => call<Subcard[]>(M('Subcards'), id)
export const getExePath = () => call<string>(M('ExePath'))
export const taskScript = (task: string) => call<string>(M('TaskScript'), task)
export const getDebug = () => call<Record<string, any>>(M('GetDebug'))

export interface PtyData { id: string; data: string }
export const onPtyData = (cb: (d: PtyData) => void) => Events.On('pty:data', (e: any) => cb(e.data))
export const onPtyExit = (cb: (d: any) => void) => Events.On('pty:exit', (e: any) => cb(e.data))
export const onWorkspaceChanged = (cb: () => void) => Events.On('workspace:changed', () => cb())
export const openTerminal = (id: string, script: string, rows: number, cols: number) =>
  call<string>(M('OpenTerminal'), id, script, rows, cols)
export const writeTerminal = (id: string, data: string) => call<void>(M('WriteTerminal'), id, data)
export const resizeTerminal = (id: string, cols: number, rows: number) =>
  call<void>(M('ResizeTerminal'), id, cols, rows)
export const closeTerminal = (id: string) => call<void>(M('CloseTerminal'), id)

export interface ThemeInfo {
  name: string
  bg: string
  panel: string
  sidebar: string
  text: string
  muted: string
  border: string
  brand: string
  accent: string
  cards: Record<string, string>
}

export const getThemes = () => call<ThemeInfo[]>(M('GetThemes'))
export const getTheme = () => call<ThemeInfo>(M('GetTheme'))
export const setTheme = (name: string) => call<ThemeInfo>(M('SetTheme'), name)

export interface Settings {
  open_output_dir: boolean
  commit_model: string
  ollama_endpoint: string
  ollama_model: string
  ollama_api_key: string
}

export const getSettings = () => call<Settings>(M('GetSettings'))
export const setSettings = (s: Settings) => call<void>(M('SetSettings'), s)

export interface Change { code: string; path: string; status: string }
export interface FileNode { name: string; path: string; isDir: boolean; children?: FileNode[] }

export const gitStatus = () => call<Change[]>(M('GitStatus'))
export const fileTree = () => call<FileNode[]>(M('FileTree'))
export const generateCommit = () => call<string>(M('GenerateCommit'))
export const commit = (message: string) => call<string>(M('Commit'), message)
