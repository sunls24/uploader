import { Ban, File, Files } from "lucide-react"
import { Button } from "@/components/ui/button.tsx"
import { $list } from "@/lib/store.ts"
import { useStore } from "@nanostores/react"
import { clsx } from "clsx"

export default function List() {
  const list = useStore($list)

  function onAbort(i: number) {
    list[i].abort()
    list[i].aborted = true
    $list.set([...list])
  }

  return (
    <div className="flex-1 overflow-y-auto">
      {list.length > 0 && (
        <div className="bg-secondary flex flex-col-reverse gap-2 rounded-sm p-2">
          {list.map((v, i) => (
            <div
              key={i}
              className="bg-background relative overflow-hidden rounded-sm"
            >
              <div
                style={{ width: `${v.progress}%` }}
                className={clsx(
                  "absolute h-1",
                  v.aborted
                    ? "bg-red-300"
                    : v.progress < 100
                      ? "bg-blue-300"
                      : "bg-green-300"
                )}
              />
              <div className="text-muted-foreground relative z-10 flex h-13 items-center p-2">
                {v.files.length}
                {v.files.length > 1 ? (
                  <Files className="mx-1 shrink-0" size={18} />
                ) : (
                  <File className="mx-1 shrink-0" size={18} />
                )}

                <div className="truncate">{v.files.join(" ")}</div>
                <div className="flex-1" />
                {v.progress < 100 && (
                  <span className="mx-1">{v.progress.toFixed(2)}%</span>
                )}
                {!v.aborted && v.progress < 100 && (
                  <Button variant="ghost" onClick={() => onAbort(i)}>
                    <Ban className="text-destructive" />
                  </Button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
