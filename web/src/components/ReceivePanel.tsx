import { useCallback, useEffect, useRef, useState } from "react"

import { api, APIError } from "@/lib/api"
import type { ShareInfo } from "@/lib/api"
import { formatBytes } from "@/lib/format"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent } from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"

export function ReceivePanel({ initialCode }: { initialCode?: string }) {
  const [code, setCode] = useState(initialCode ?? "")
  const [share, setShare] = useState<ShareInfo | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const lookupVersion = useRef(0)

  const lookup = useCallback(async (value: string) => {
    const version = ++lookupVersion.current
    setError(null)
    setShare(null)
    try {
      const data = await api.shareInfo(value)
      if (version === lookupVersion.current) setShare(data)
    } catch (e) {
      if (version === lookupVersion.current)
        setError(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    if (initialCode && /^\d{4,8}$/.test(initialCode)) void lookup(initialCode)
  }, [initialCode, lookup])

  useEffect(() => {
    if (!share || share.done || share.failed) return
    let alive = true
    const tick = async () => {
      try {
        const data = await api.shareInfo(share.code)
        if (alive) {
          setShare(data)
          setError(null)
        }
      } catch (e) {
        if (!alive) return
        if (e instanceof APIError && (e.status === 404 || e.status === 410)) {
          setShare(null)
          setError(e.message)
        }
      }
    }
    const id = setInterval(tick, 1500)
    return () => {
      alive = false
      clearInterval(id)
    }
  }, [share?.code, share?.done, share?.failed])

  async function receive() {
    if (!share) return
    setBusy(true)
    setError(null)
    try {
      await api.claim(share.code)
      const a = document.createElement("a")
      a.href = `/api/shares/${share.code}/download`
      a.download = share.name
      document.body.append(a)
      a.click()
      a.remove()
    } catch (e) {
      if (e instanceof APIError && e.status === 409)
        setError("已被其他接收者领取")
      else setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const status = () => {
    if (!share) return null
    if (share.failed)
      return { label: "上传失败", variant: "destructive" as const }
    if (share.done) return { label: "可以接收", variant: "success" as const }
    const percent =
      share.size > 0 ? Math.round((share.written / share.size) * 100) : 0
    return {
      label: share.claimed ? `接收中 ${percent}%` : "等待发送",
      variant: "warning" as const,
    }
  }

  const badge = status()

  return (
    <div className="space-y-4">
      <form
        className="flex gap-2"
        onSubmit={(e) => {
          e.preventDefault()
          void lookup(code)
        }}
      >
        <Input
          aria-label="分享码"
          value={code}
          onChange={(e) => setCode(e.target.value)}
          inputMode="numeric"
          pattern="[0-9]{4,8}"
          minLength={4}
          maxLength={8}
          required
          placeholder="分享码"
          className="font-mono tracking-[0.2em]"
        />
        <Button type="submit">查看</Button>
      </form>
      {badge && share && (
        <Card>
          <CardContent className="space-y-3">
            <div className="flex items-center justify-between gap-2">
              <span className="min-w-0 truncate text-sm font-medium">
                {share.name}
              </span>
              <Badge variant={badge.variant}>{badge.label}</Badge>
            </div>
            {!share.done && !share.failed && (
              <Progress
                value={share.size > 0 ? (share.written / share.size) * 100 : 0}
              />
            )}
            <p className="text-muted-foreground text-xs">
              {formatBytes(share.size)} · 有效期至{" "}
              {new Date(share.expiresAt).toLocaleString()}
            </p>
            <Button
              size="sm"
              disabled={share.failed || busy}
              onClick={() => void receive()}
            >
              接收
            </Button>
          </CardContent>
        </Card>
      )}
      {error && <p className="text-destructive text-sm">{error}</p>}
    </div>
  )
}
