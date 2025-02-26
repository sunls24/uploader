import { CloudUpload, Hand } from "lucide-react"
import React, { useRef, useState } from "react"
import { cn, uploadFiles } from "@/lib/utils.ts"
import { toast } from "sonner"

export default function Uploader() {
  const [dragging, setDragging] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  function onDragLeave(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault()
    e.stopPropagation()
    setDragging(false)
  }

  function onDragEnter(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault()
    e.stopPropagation()
    setDragging(true)
  }

  function onDragOver(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault()
    e.stopPropagation()
  }

  async function onDrop(e: React.DragEvent<HTMLDivElement>) {
    e.preventDefault()
    e.stopPropagation()
    setDragging(false)

    const files: string[] = []
    const formData = new FormData()
    for (let i = 0; i < e.dataTransfer.files.length; i++) {
      const file = e.dataTransfer.files[i]
      if (e.dataTransfer.items[i].webkitGetAsEntry()?.isDirectory) {
        toast.info(`忽略文件夹: ${file.name}`)
        continue
      }
      files.push(file.name)
      formData.append("files", file)
    }
    if (files.length == 0) {
      return
    }

    uploadFiles(formData, files)
  }

  function onFileChange(e: React.ChangeEvent<HTMLInputElement>) {
    e.preventDefault()
    e.stopPropagation()

    const list = e.currentTarget.files
    if (!list || !list.length) {
      return
    }
    const files: string[] = []
    const formData = new FormData()
    for (const f of list) {
      formData.append("files", f)
      files.push(f.name)
    }
    uploadFiles(formData, files)
  }

  function onClick() {
    fileRef.current?.click()
  }

  return (
    <div
      onClick={onClick}
      onDragLeave={onDragLeave}
      onDragEnter={onDragEnter}
      onDragOver={onDragOver}
      onDrop={onDrop}
      className={cn(
        "bg-secondary text-muted-foreground flex h-40 w-full items-center justify-center rounded-sm hover:cursor-pointer",
        dragging && "text-foreground border-4"
      )}
    >
      {dragging ? (
        <Hand className="mr-1" size={20} />
      ) : (
        <CloudUpload className="mr-1" size={20} />
      )}
      {dragging ? "松手即开始上传" : "点击或拖拽上传"}
      <input
        multiple
        type="file"
        ref={fileRef}
        className="hidden"
        onChange={onFileChange}
      />
    </div>
  )
}
