import { useState, useRef, useEffect, useCallback } from 'react'

const PAGE = 100

export function useInfiniteScroll<T>(items: T[]): {
  visible: T[]
  sentinelRef: React.RefObject<HTMLDivElement | null>
} {
  const [pageSize, setPageSize] = useState(PAGE)
  const sentinelRef = useRef<HTMLDivElement>(null)

  const loadMore = useCallback(() => setPageSize(p => p + PAGE), [])

  useEffect(() => {
    setPageSize(PAGE)
  }, [items])

  useEffect(() => {
    const el = sentinelRef.current
    if (!el) return
    const obs = new IntersectionObserver(
      entries => { if (entries[0].isIntersecting) loadMore() },
      { rootMargin: '200px' },
    )
    obs.observe(el)
    return () => obs.disconnect()
  }, [loadMore])

  return { visible: items.slice(0, pageSize), sentinelRef }
}
