import type { InputHTMLAttributes } from "react"

export function Input({
  className = "",
  ...props
}: InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={`border-input placeholder:text-muted-foreground focus-visible:outline-ring/60 h-9 min-w-0 rounded-md border bg-transparent px-3 py-1 text-sm transition-colors focus-visible:outline-2 focus-visible:outline-offset-1 disabled:cursor-not-allowed disabled:opacity-50 ${className}`}
      {...props}
    />
  )
}
