import type { HTMLAttributes } from "react"

export function Card({
  className = "",
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={`border-border bg-card text-card-foreground rounded-lg border ${className}`}
      {...props}
    />
  )
}

export function CardContent({
  className = "",
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={`p-4 ${className}`} {...props} />
}
