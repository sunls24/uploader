export type Entry = { name: string; directory: boolean; size: number }

export type ShareInfo = {
  code: string
  name: string
  size: number
  written: number
  done: boolean
  failed: boolean
  claimed: boolean
  expiresAt: string
}

export class APIError extends Error {
  constructor(
    message: string,
    public status: number
  ) {
    super(message)
  }
}

export async function request(url: string, init?: RequestInit) {
  const res = await fetch(url, init)
  if (!res.ok) {
    let message = `请求失败 ${res.status}`
    try {
      const body = await res.json()
      if (body.message) message = body.message
    } catch {
      // 非 JSON 错误响应，保留默认消息
    }
    throw new APIError(message, res.status)
  }
  return res
}

export const api = {
  session: () => request("/api/auth").then((r) => r.json()),

  login: (password: string) =>
    request("/api/auth", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ password }),
    }).then((r) => r.json()),

  list: (path: string) =>
    request(`/api/files?path=${encodeURIComponent(path)}`).then(
      (r) => r.json() as Promise<Entry[]>
    ),

  remove: (path: string) =>
    request(`/api/files?path=${encodeURIComponent(path)}`, {
      method: "DELETE",
    }),

  createShare: (name: string, size: number) =>
    request("/api/shares", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name, size }),
    }).then((r) => r.json() as Promise<{ code: string; owner: string }>),

  shareInfo: (code: string) =>
    request(`/api/shares/${code}`).then((r) => r.json() as Promise<ShareInfo>),

  claim: (code: string) =>
    request(`/api/shares/${code}/claim`, { method: "POST" }),
}

export function upload(
  url: string,
  file: File,
  onProgress: (percent: number) => void,
  owner?: string
) {
  const xhr = new XMLHttpRequest()
  const promise = new Promise<void>((resolve, reject) => {
    xhr.open("PUT", url)
    if (owner) xhr.setRequestHeader("X-Share-Owner", owner)
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100))
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) return resolve()
      try {
        reject(new APIError(JSON.parse(xhr.responseText).message, xhr.status))
      } catch {
        reject(new APIError(`上传失败 ${xhr.status}`, xhr.status))
      }
    }
    xhr.onerror = () => reject(new Error("网络中断，请重新上传"))
    xhr.onabort = () => reject(new Error("上传已取消"))
    xhr.send(file)
  })
  return { promise, cancel: () => xhr.abort() }
}
