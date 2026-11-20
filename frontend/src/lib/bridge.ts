import { Call, Events } from '@wailsio/runtime'

export interface ProjectInfo { path: string; name: string; type: string }
export interface MenuItemInfo {
  id: string
  label: string
  description: string
  icon: string
  color: string
  section: string
  platform: 'windows' | 'android' | 'general'
}
export interface WorkspaceInfo { projects: ProjectInfo[]; active: number }

const call = <T>(method: string, ...args: any[]): Promise<T> =>
  Call.ByName(method, ...args) as unknown as Promise<T>

const M = (name: string) => `compass/internal/webui.Bridge.${name}`

export const getWorkspace = () => call<WorkspaceInfo>(M('GetWorkspace'))
export const selectProject = (i: number) => call<WorkspaceInfo>(M('SelectProject'), i)
export const removeProject = (i: number) => call<WorkspaceInfo>(M('RemoveProject'), i)
export const pickProject = () => call<WorkspaceInfo>(M('PickProject'))
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
export const openTerminal = (id: string, label: string, script: string, rows: number, cols: number) =>
  call<string>(M('OpenTerminal'), id, label, script, rows, cols)
export const writeTerminal = (id: string, data: string) => call<void>(M('WriteTerminal'), id, data)
export const resizeTerminal = (id: string, cols: number, rows: number) =>
  call<void>(M('ResizeTerminal'), id, cols, rows)
export const closeTerminal = (id: string) => call<void>(M('CloseTerminal'), id)
export interface TerminalInfo { id: string; label: string; project: string; exited: boolean; output: string }
export const listTerminals = () => call<TerminalInfo[]>(M('ListTerminals'))
export const focusWindow = () => call<void>(M('FocusWindow'))

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
  close_tab_on_build_end: boolean
  openrouter_endpoint: string
  openrouter_model: string
  openrouter_api_key: string
}

export const getSettings = () => call<Settings>(M('GetSettings'))
export const setSettings = (s: Settings) => call<void>(M('SetSettings'), s)

export interface ModelOption { id: string; name: string; context: number }
export const listModels = () => call<ModelOption[]>(M('ListModels'))

export interface Change { code: string; path: string; status: string }
export interface FileNode { name: string; path: string; rel: string; isDir: boolean; children?: FileNode[] }

export const gitStatus = () => call<Change[]>(M('GitStatus'))
export const listDir = (dir: string) => call<FileNode[]>(M('ListDir'), dir)
export const fileDelete = (path: string) => call<void>(M('FileDelete'), path)
export const fileRename = (path: string, name: string) => call<string>(M('FileRename'), path, name)
export const fileCopy = (paths: string[], dest: string) => call<void>(M('FileCopy'), paths, dest)

export interface CleanupTarget { label: string; path: string; size: string; deps: boolean }
export interface CleanupPurgeItem { label: string; path: string; freed: number; error: string }
export interface CleanupPurgeResult { freed: number; freed_label: string; items: CleanupPurgeItem[]; errors: string[] }
export const listCleanupCandidates = () => call<CleanupTarget[]>(M('ListCleanupCandidates'))
export const purgeCleanupCandidates = (paths: string[]) => call<CleanupPurgeResult>(M('PurgeCleanupCandidates'), paths)
export const fileMove = (paths: string[], dest: string) => call<void>(M('FileMove'), paths, dest)
export const generateCommit = (dir: string) => call<string>(M('GenerateCommit'), dir)
export const cancelCommit = (dir: string) => call<void>(M('CancelCommit'), dir)
export const commit = (dir: string, message: string) => call<string>(M('Commit'), dir, message)

export interface CommitEntry { hash: string; short: string; author: string; date: string; subject: string }
export const gitLog = () => call<CommitEntry[]>(M('GitLog'))
export const gitBranch = () => call<string>(M('GitBranch'))

export interface GitInfo {
  isRepo: boolean
  hasCommits: boolean
  branch: string
  upstream: string
  remoteUrl: string
  webUrl: string
  isGitHub: boolean
  ahead: number
  behind: number
  changes: number
}
export const gitInfo = () => call<GitInfo>(M('GitInfo'))
export const gitPush = () => call<string>(M('GitPush'))
export const gitPull = () => call<string>(M('GitPull'))
export const gitSetRemote = (url: string) => call<string>(M('GitSetRemote'), url)
export const gitInitRepo = () => call<string>(M('GitInitRepo'))
export const openExternal = (url: string) => call<void>(M('OpenExternal'), url)

export interface CommitFile { path: string; status: string; added: number; removed: number; binary: boolean }
export interface CommitDetail {
  hash: string
  short: string
  author: string
  date: string
  subject: string
  body: string
  files: CommitFile[]
}
export const commitDetail = (hash: string) => call<CommitDetail>(M('CommitDetail'), hash)
