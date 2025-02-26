import { type ClassValue, clsx } from "clsx"
import { twMerge } from "tailwind-merge"
import { $list, $path } from "@/lib/store.ts"
import { toast } from "sonner"

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

export function uploadFiles(formData: FormData, files: string[]) {
  const xhr = new XMLHttpRequest()
  xhr.open("POST", `/api/upload?dir=${$path.get()}`)

  let index: number
  xhr.onloadstart = () => {
    const list = $list.get()
    index = list.length
    $list.set([
      ...list,
      { files, progress: 0, abort: () => xhr.abort(), aborted: false },
    ])
  }

  xhr.upload.onprogress = function (e) {
    if (!e.lengthComputable) {
      return
    }
    const list = $list.get()
    list[index].progress = (e.loaded / e.total) * 100
    $list.set([...list])
  }

  xhr.onload = function () {
    if (xhr.status === 200) {
      if (files.length > 1) {
        toast.success(`共${files.length}个文件上传成功`)
      } else {
        toast.success(`${files[0]} 上传成功`)
      }
      return
    }
    try {
      toast.error(JSON.parse(xhr.responseText).message)
    } catch {
      toast.error(`${xhr.statusText}: ${xhr.responseText}`)
    }
  }

  xhr.send(formData)
}
