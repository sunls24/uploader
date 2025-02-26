import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select.tsx"
import { useEffect, useState } from "react"
import { toast } from "sonner"
import { $path } from "@/lib/store.ts"

export default function Path() {
  const [pathDirs, setPathDirs] = useState<string[][]>([])
  const [paths, setPaths] = useState<string[]>([])

  useEffect(() => {
    const path = paths.join("/")
    $path.set(path)
    fetch(`/api/dir?p=${path}`)
      .then((res) =>
        res.ok ? res.json() : res.json().then((j) => Promise.reject(j.message))
      )
      .then((res) => res.length && setPathDirs([...pathDirs, res]))
      .catch((err) => toast.error(err.message ?? err))
  }, [paths])

  function onSelect(value: string, i: number) {
    paths[i] = value
    if (i < paths.length - 1) {
      setPathDirs(pathDirs.slice(0, i + 1))
    }
    setPaths(paths.splice(0, i + 1))
  }

  return (
    <div className="bg-secondary flex w-full items-center gap-2 rounded-sm px-3 py-2">
      <div className="bg-background/50 border-border/70 text-muted-foreground/50 flex h-9 items-center rounded-sm border px-2">
        /
      </div>
      {pathDirs.map((dirs, i) => (
        <Select key={i} onValueChange={(v) => onSelect(v, i)}>
          <SelectTrigger className="bg-background w-fit">
            <SelectValue placeholder="DIR" />
          </SelectTrigger>
          <SelectContent>
            <SelectGroup>
              {dirs.map((dir, i) => (
                <SelectItem key={i} value={dir}>
                  {dir}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      ))}
    </div>
  )
}
