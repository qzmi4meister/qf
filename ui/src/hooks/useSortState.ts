import { useState } from 'react'

type SortDir = 'asc' | 'desc'

export interface SortState {
  key: string | null
  dir: SortDir
}

export interface UseSortStateReturn {
  sort: SortState
  toggle: (key: string) => void
  sorted: <T>(data: T[], accessor: (row: T, key: string) => string | number | null | undefined) => T[]
}

export function useSortState(initial?: { key: string; dir: SortDir }): UseSortStateReturn {
  const [sort, setSort] = useState<SortState>(
    initial ?? { key: null, dir: 'asc' }
  )

  function toggle(key: string) {
    setSort(s => {
      if (s.key !== key) return { key, dir: 'asc' }
      return { key, dir: s.dir === 'asc' ? 'desc' : 'asc' }
    })
  }

  function sorted<T>(
    data: T[],
    accessor: (row: T, key: string) => string | number | null | undefined,
  ): T[] {
    if (!sort.key) return data
    const k = sort.key
    return [...data].sort((a, b) => {
      const av = accessor(a, k)
      const bv = accessor(b, k)
      if (av == null && bv == null) return 0
      if (av == null) return 1
      if (bv == null) return -1
      const cmp = av < bv ? -1 : av > bv ? 1 : 0
      return sort.dir === 'asc' ? cmp : -cmp
    })
  }

  return { sort, toggle, sorted }
}
