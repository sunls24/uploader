import { useCallback, useEffect, useState } from "react"

import { api } from "@/lib/api"

export function useSession() {
  const [expiresAt, setExpiresAt] = useState<number | null>(null)
  const [ready, setReady] = useState(false)
  const [remaining, setRemaining] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    api
      .session()
      .then((session: { expiresAt: string }) => {
        if (!controller.signal.aborted)
          setExpiresAt(Date.parse(session.expiresAt))
      })
      .catch(() => {
        if (!controller.signal.aborted) setExpiresAt(null)
      })
      .finally(() => {
        if (!controller.signal.aborted) setReady(true)
      })
    return () => controller.abort()
  }, [])

  useEffect(() => {
    if (expiresAt === null) return
    const timer = setTimeout(
      () => setExpiresAt(null),
      Math.max(0, expiresAt - Date.now())
    )
    return () => clearTimeout(timer)
  }, [expiresAt])

  useEffect(() => {
    if (expiresAt === null) return
    const update = () => setRemaining(Math.max(0, expiresAt - Date.now()))
    update()
    const id = setInterval(update, 1000)
    return () => clearInterval(id)
  }, [expiresAt])

  const invalidate = useCallback(() => {
    setExpiresAt(null)
  }, [])

  const login = useCallback(async (password: string) => {
    const session = (await api.login(password)) as { expiresAt: string }
    setExpiresAt(Date.parse(session.expiresAt))
  }, [])

  return {
    ready,
    unlocked: expiresAt !== null && remaining > 0,
    remaining,
    login,
    invalidate,
  }
}
