import { useEffect, useRef, useState } from "react"

import { useSession } from "@/hooks/useSession"
import { formatRemaining } from "@/lib/format"
import { AuthDialog } from "@/components/AuthDialog"
import { FileBrowser } from "@/components/FileBrowser"
import { ReceivePanel } from "@/components/ReceivePanel"
import { SendPanel } from "@/components/SendPanel"
import { Badge } from "@/components/ui/badge"

type Tab = "files" | "send" | "receive"

const tabs: { key: Tab; label: string }[] = [
  { key: "files", label: "文件库" },
  { key: "send", label: "临时发送" },
  { key: "receive", label: "接收文件" },
]

export default function FilesApp() {
  const session = useSession()
  const [tab, setTab] = useState<Tab>("files")
  const [authOpen, setAuthOpen] = useState(false)
  const [authBusy, setAuthBusy] = useState(false)
  const [authError, setAuthError] = useState<string | null>(null)
  const pending = useRef<(() => void) | null>(null)
  const [initialCode, setInitialCode] = useState("")

  useEffect(() => {
    const value = new URLSearchParams(location.search).get("share") ?? ""
    if (value && /^\d{4,8}$/.test(value)) {
      setInitialCode(value)
      setTab("receive")
    }
  }, [])

  // 按需鉴权：已解锁直接执行；否则记录待执行操作并弹出密码框，验证通过后自动继续。
  const guard = (action: () => void) => {
    if (session.unlocked) {
      action()
      return
    }
    pending.current = action
    setAuthOpen(true)
  }

  const cancelAuth = () => {
    pending.current = null
    setAuthOpen(false)
    setAuthError(null)
  }

  async function submitAuth(password: string) {
    setAuthBusy(true)
    setAuthError(null)
    try {
      await session.login(password)
      setAuthOpen(false)
      const action = pending.current
      pending.current = null
      action?.()
    } catch (err) {
      setAuthError(err instanceof Error ? err.message : String(err))
    } finally {
      setAuthBusy(false)
    }
  }

  return (
    <>
      <div className="space-y-5" inert={authOpen}>
        <header className="flex items-center justify-between gap-3">
          <div>
            <h1 className="text-xl font-semibold">文件站</h1>
          </div>
          {session.unlocked && (
            <Badge variant="success" className="shrink-0">
              已解锁 · 剩余 {formatRemaining(session.remaining)}
            </Badge>
          )}
        </header>

        <nav className="border-border flex gap-1 border-b">
          {tabs.map((t) => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className={`-mb-px rounded-t-md border-b-2 px-3 py-2 text-sm transition-colors ${
                tab === t.key
                  ? "border-primary text-foreground font-medium"
                  : "text-muted-foreground hover:text-foreground border-transparent"
              }`}
            >
              {t.label}
            </button>
          ))}
        </nav>

        {tab === "files" && (
          <FileBrowser guard={guard} onUnauthorized={session.invalidate} />
        )}
        {tab === "send" && (
          <SendPanel guard={guard} onUnauthorized={session.invalidate} />
        )}
        {tab === "receive" && <ReceivePanel initialCode={initialCode} />}
      </div>

      <AuthDialog
        open={authOpen}
        busy={authBusy}
        error={authError}
        onSubmit={submitAuth}
        onCancel={cancelAuth}
      />
    </>
  )
}
