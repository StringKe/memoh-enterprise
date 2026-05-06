export interface FsFileInfo {
  name?: string;
  path?: string;
  isDir?: boolean;
  size?: number;
  modTime?: string;
}

export interface FsReadResponse {
  content?: string;
}
