import { useCallback, useEffect, useRef, useState } from "react"

import { api, APIError, upload } from "@/lib/api"
import type { Entry } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { Button } from "@/components/ui/button"
import { Progress } from "@/components/ui/progress"

type UploadItem = {
  key: string
  name: string
  progress: number
  error?: string
  cancel?: () => void
}

function FolderIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="size-4 shrink-0 fill-amber-500/80"
      aria-hidden
    >
      <path d="M4 4h6l2 2h8a1 1 0 0 1 1 1v11a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2Z" />
    </svg>
  )
}

function FileIcon() {
  return (
    <svg
      viewBox="0 0 24 24"
      className="text-muted-foreground size-4 shrink-0 fill-current"
      aria-hidden
    >
      <path d="M6 2h8l6 6v12a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2Zm8 2v5h5Z" />
    </svg>
  )
}

export function FileBrowser({
  guard,
  onUnauthorized,
}: {
  guard: (action: () => void) => void
  onUnauthorized: () => void
}) {
  const [path, setPath] = useState("")
  const [entries, setEntries] = useState<Entry[]>([])
  const [loading, setLoading] = useState(true)
  const [listError, setListError] = useState<string | null>(null)
  const [revision, setRevision] = useState(0)
  const [uploads, setUploads] = useState<UploadItem[]>([])
  const [dragging, setDragging] = useState(false)
  const [confirming, setConfirming] = useState<string | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)
  const handleError = useCallback(
    (e: unknown) => {
      if (e instanceof APIError && e.status === 401) onUnauthorized()
      return e instanceof Error ? e.message : String(e)
    },
    [onUnauthorized]
  )

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setListError(null)
    api
      .list(path)
      .then((items) => {
        if (!controller.signal.aborted) setEntries(items)
      })
      .catch((e) => {
        if (!controller.signal.aborted) setListError(handleError(e))
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [path, revision, handleError])

  const fullPath = (name: string) => (path ? `${path}/${name}` : name)
  const refresh = () => setRevision((n) => n + 1)

  async function sendFiles(files: File[]) {
    for (let i = 0; i < files.length; i++) {
      const file = files[i]
      const key = `${file.name}-${Date.now()}-${Math.random()}`
      const { promise, cancel } = upload(
        `/api/files?path=${encodeURIComponent(fullPath(file.name))}`,
        file,
        (percent) =>
          setUploads((items) =>
            items.map((item) =>
              item.key === key ? { ...item, progress: percent } : item
            )
          )
      )
      setUploads((items) => [
        ...items,
        { key, name: file.name, progress: 0, cancel },
      ])
      try {
        await promise
        setUploads((items) =>
          items.map((item) =>
            item.key === key ? { ...item, progress: 100 } : item
          )
        )
        setTimeout(
          () => setUploads((items) => items.filter((item) => item.key !== key)),
          1200
        )
      } catch (e) {
        const message = e instanceof Error ? e.message : String(e)
        if (message === "上传已取消") {
          setUploads((items) => items.filter((item) => item.key !== key))
          continue
        }
        if (e instanceof APIError && e.status === 401) {
          // 会话过期：移除当前行，刷新已上传部分，验证后从当前文件继续剩余队列。
          setUploads((items) => items.filter((item) => item.key !== key))
          refresh()
          onUnauthorized()
          guard(() => void sendFiles(files.slice(i)))
          return
        }
        setUploads((items) =>
          items.map((item) =>
            item.key === key ? { ...item, error: message } : item
          )
        )
      }
    }
    refresh()
  }

  function onDelete(name: string) {
    if (confirming !== name) {
      setConfirming(name)
      setTimeout(() => setConfirming((c) => (c === name ? null : c)), 3000)
      return
    }
    setConfirming(null)
    const doRemove = () =>
      api
        .remove(fullPath(name))
        .then(refresh)
        .catch((e) => {
          if (e instanceof APIError && e.status === 401) {
            onUnauthorized()
            guard(doRemove)
            return
          }
          setListError(e instanceof Error ? e.message : String(e))
        })
    guard(doRemove)
  }

  const segments = path.split("/").filter(Boolean)

  return (
    <div className="space-y-4">
      <div
        role="button"
        tabIndex={0}
        onClick={() => fileInput.current?.click()}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") fileInput.current?.click()
        }}
        onDragOver={(e) => {
          e.preventDefault()
          setDragging(true)
        }}
        onDragLeave={() => setDragging(false)}
        onDrop={(e) => {
          e.preventDefault()
          setDragging(false)
          guard(() => void sendFiles([...e.dataTransfer.files]))
        }}
        className={`cursor-pointer rounded-lg border-2 border-dashed p-6 text-center text-sm transition-colors ${
          dragging
            ? "border-ring bg-accent"
            : "border-border bg-secondary/50 hover:border-ring/60"
        }`}
      >
        拖拽或点击上传
        <input
          ref={fileInput}
          type="file"
          multiple
          className="hidden"
          onChange={(e) => {
            guard(() => void sendFiles([...(e.target.files ?? [])]))
            e.target.value = ""
          }}
        />
      </div>

      {uploads.length > 0 && (
        <ul className="space-y-2">
          {uploads.map((item) => (
            <li
              key={item.key}
              className="border-border bg-card rounded-lg border p-3"
            >
              <div className="mb-1.5 flex items-center gap-2 text-sm">
                <span className="min-w-0 flex-1 truncate">{item.name}</span>
                {item.error ? (
                  <span className="text-destructive text-xs">{item.error}</span>
                ) : (
                  <span className="text-muted-foreground text-xs">
                    {item.progress}%
                  </span>
                )}
                {item.error ? (
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() =>
                      setUploads((items) =>
                        items.filter((i) => i.key !== item.key)
                      )
                    }
                  >
                    关闭
                  </Button>
                ) : (
                  item.progress < 100 && (
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => item.cancel?.()}
                    >
                      取消
                    </Button>
                  )
                )}
              </div>
              {!item.error && <Progress value={item.progress} />}
            </li>
          ))}
        </ul>
      )}

      <div className="flex items-center gap-1 text-sm">
        <button
          className="hover:bg-accent rounded px-1 py-0.5"
          onClick={() => setPath("")}
        >
          全部文件
        </button>
        {segments.map((segment, i) => (
          <span key={i} className="flex items-center gap-1">
            <span className="text-muted-foreground">/</span>
            <button
              className="hover:bg-accent rounded px-1 py-0.5"
              onClick={() => setPath(segments.slice(0, i + 1).join("/"))}
            >
              {segment}
            </button>
          </span>
        ))}
        <span className="text-muted-foreground ml-auto text-xs">
          {loading ? "加载中…" : `${entries.length} 项`}
        </span>
      </div>

      {listError && <p className="text-destructive text-sm">{listError}</p>}

      <div className="border-border overflow-hidden rounded-lg border">
        {loading ? (
          <div className="text-muted-foreground space-y-2 p-4 text-sm">
            <div className="bg-secondary h-4 w-1/3 animate-pulse rounded" />
            <div className="bg-secondary h-4 w-1/2 animate-pulse rounded" />
            <div className="bg-secondary h-4 w-2/5 animate-pulse rounded" />
          </div>
        ) : entries.length === 0 ? (
          <p className="text-muted-foreground p-6 text-center text-sm">
            暂无文件
          </p>
        ) : (
          <ul className="divide-border divide-y">
            {entries.map((item) => (
              <li
                key={item.name}
                className="group flex items-center gap-3 px-3 py-2.5 text-sm"
              >
                {item.directory ? <FolderIcon /> : <FileIcon />}
                {item.directory ? (
                  <button
                    className="min-w-0 flex-1 truncate text-left hover:underline"
                    onClick={() => setPath(fullPath(item.name))}
                  >
                    {item.name}
                  </button>
                ) : (
                  <a
                    className="min-w-0 flex-1 truncate hover:underline"
                    href={`/api/download?path=${encodeURIComponent(fullPath(item.name))}`}
                    download={item.name}
                  >
                    {item.name}
                  </a>
                )}
                <span className="text-muted-foreground w-20 shrink-0 text-right text-xs">
                  {item.directory ? "目录" : formatBytes(item.size)}
                </span>
                {item.directory ? (
                  <span className="w-14" />
                ) : (
                  <span className="flex w-14 justify-end">
                    {confirming === item.name ? (
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => onDelete(item.name)}
                      >
                        确认
                      </Button>
                    ) : (
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-muted-foreground hover:text-destructive opacity-100 md:opacity-0 md:group-hover:opacity-100"
                        onClick={() => onDelete(item.name)}
                      >
                        删除
                      </Button>
                    )}
                  </span>
                )}
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  )
}
