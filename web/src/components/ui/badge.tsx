import type { HTMLAttributes } from "react"

type Variant = "success" | "warning" | "destructive"

const variants: Record<Variant, string> = {
  success: "bg-green-600/10 text-green-700 dark:text-green-400",
  warning: "bg-amber-600/10 text-amber-700 dark:text-amber-400",
  destructive: "bg-destructive/10 text-destructive",
}

export function Badge({
  variant,
  className = "",
  ...props
}: HTMLAttributes<HTMLSpanElement> & { variant: Variant }) {
  return (
    <span
      className={`inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium ${variants[variant]} ${className}`}
      {...props}
    />
  )
}
