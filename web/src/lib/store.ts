import { atom } from "nanostores"

export const $path = atom("")

type Item = {
  files: string[]
  progress: number
  abort: () => void
  aborted: boolean
}

export const $list = atom<Item[]>([])
