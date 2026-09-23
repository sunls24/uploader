import { useEffect, useState } from "react"

import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"

export function AuthDialog({
  open,
  busy,
  error,
  onSubmit,
  onCancel,
}: {
  open: boolean
  busy: boolean
  error: string | null
  onSubmit: (password: string) => void
  onCancel: () => void
}) {
  const [password, setPassword] = useState("")

  useEffect(() => {
    if (!open) return
    setPassword("")
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onCancel()
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  }, [open, onCancel])

  if (!open) return null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div
        className="absolute inset-0 bg-black/50"
        onClick={onCancel}
        aria-hidden
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label="需要管理密码"
        className="border-border bg-card relative w-full max-w-sm rounded-lg border p-4"
      >
        <h2 className="text-sm font-medium">需要管理密码</h2>
        <form
          className="mt-3"
          onSubmit={(e) => {
            e.preventDefault()
            if (password) onSubmit(password)
          }}
        >
          <div className="flex items-center gap-2">
            <Input
              aria-label="管理密码"
              type="password"
              autoComplete="current-password"
              autoFocus
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="管理密码"
              required
              disabled={busy}
              className="min-w-0 flex-1"
            />
            <Button variant="ghost" onClick={onCancel} disabled={busy}>
              取消
            </Button>
            <Button type="submit" disabled={busy}>
              {busy ? "验证中…" : "解锁"}
            </Button>
          </div>
          {error && <p className="text-destructive mt-2 text-sm">{error}</p>}
        </form>
      </div>
    </div>
  )
}
