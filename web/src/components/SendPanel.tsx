import { useEffect, useRef, useState } from "react"

import { api, APIError, upload } from "@/lib/api"
import type { ShareInfo } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"

export function SendPanel({
  guard,
  onUnauthorized,
}: {
  guard: (action: () => void) => void
  onUnauthorized: () => void
}) {
  const [code, setCode] = useState<string | null>(null)
  const [info, setInfo] = useState<ShareInfo | null>(null)
  const [localProgress, setLocalProgress] = useState(0)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const cancelRef = useRef<(() => void) | null>(null)
  const fileInput = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!code || info?.done || info?.failed) return
    let alive = true
    const tick = async () => {
      try {
        const data = await api.shareInfo(code)
        if (alive) setInfo(data)
      } catch (e) {
        if (!alive) return
        if (e instanceof APIError && (e.status === 404 || e.status === 410)) {
          setCode(null)
          setInfo(null)
          setError("分享已过期")
        }
      }
    }
    const id = setInterval(tick, 2000)
    return () => {
      alive = false
      clearInterval(id)
    }
  }, [code, info?.done, info?.failed])

  useEffect(() => () => cancelRef.current?.(), [])

  async function send(file: File) {
    setError(null)
    setLocalProgress(0)
    setBusy(true)
    try {
      const created = await api.createShare(file.name, file.size)
      setCode(created.code)
      const { promise, cancel } = upload(
        `/api/shares/${created.code}`,
        file,
        setLocalProgress,
        created.owner
      )
      cancelRef.current = cancel
      try {
        await promise
      } finally {
        cancelRef.current = null
      }
    } catch (e) {
      if (e instanceof APIError && e.status === 401) onUnauthorized()
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  function reset() {
    cancelRef.current?.()
    setCode(null)
    setInfo(null)
    setLocalProgress(0)
    setError(null)
    if (fileInput.current) fileInput.current.value = ""
  }

  const copyLink = async () => {
    if (!code) return
    await navigator.clipboard.writeText(`${location.origin}/?share=${code}`)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const status = () => {
    if (!info) return busy ? "创建分享…" : ""
    if (info.failed) return "上传失败"
    if (info.done) return "上传完成"
    return `上传中 ${localProgress}%`
  }

  return (
    <div className="space-y-4">
      {code ? (
        <Card>
          <CardContent className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-muted-foreground text-sm">分享码</span>
              {info && (
                <Badge
                  variant={
                    info.failed
                      ? "destructive"
                      : info.done
                        ? "success"
                        : "warning"
                  }
                >
                  {status()}
                </Badge>
              )}
            </div>
            <p className="font-mono text-3xl font-medium tracking-[0.3em]">
              {code}
            </p>
            {info && !info.failed && !info.done && (
              <Progress value={localProgress} />
            )}
            {info && (
              <p className="text-muted-foreground text-xs break-all">
                {info.name} · {formatBytes(info.size)} ·{" "}
                {info.claimed ? "已领取" : "未领取"}
                {info.expiresAt &&
                  ` · 有效期至 ${new Date(info.expiresAt).toLocaleString()}`}
              </p>
            )}
            <div className="flex gap-2">
              <Button size="sm" onClick={() => void copyLink()}>
                {copied ? "已复制" : "复制接收链接"}
              </Button>
              <Button size="sm" variant="outline" onClick={reset}>
                发送新文件
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : (
        <label
          className={`border-border hover:border-ring/60 flex cursor-pointer items-center gap-3 rounded-lg border border-dashed p-4 text-sm transition-colors ${
            busy && "pointer-events-none opacity-60"
          }`}
        >
          <span className="text-muted-foreground">选择文件</span>
          <input
            ref={fileInput}
            type="file"
            className="min-w-0 flex-1 text-xs"
            disabled={busy}
            onChange={(e) => {
              const file = e.target.files?.[0]
              e.target.value = ""
              if (file) guard(() => void send(file))
            }}
          />
        </label>
      )}
      {error && <p className="text-destructive text-sm">{error}</p>}
    </div>
  )
}
