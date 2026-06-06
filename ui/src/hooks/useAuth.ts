import { useQuery } from '@tanstack/react-query'
import { getMe } from '../api/auth'
import type { Me } from '../types'

export function useAuth(): { user: Me | null; isLoading: boolean; isAuthenticated: boolean } {
  const { data, isLoading } = useQuery({
    queryKey: ['me'],
    queryFn: getMe,
    retry: false,
    staleTime: 5 * 60 * 1000,
  })
  return {
    user: data ?? null,
    isLoading,
    isAuthenticated: !!data,
  }
}
