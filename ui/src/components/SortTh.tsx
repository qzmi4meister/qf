import { Table, Group, Text } from '@mantine/core'
import type { SortState } from '../hooks/useSortState'

interface Props {
  sortKey: string
  sort: SortState
  onSort: (key: string) => void
  children: React.ReactNode
  style?: React.CSSProperties
}

export function SortTh({ sortKey, sort, onSort, children, style }: Props) {
  const active = sort.key === sortKey
  const icon = active ? (sort.dir === 'asc' ? '↑' : '↓') : '↕'
  return (
    <Table.Th
      onClick={() => onSort(sortKey)}
      style={{ cursor: 'pointer', userSelect: 'none', ...style }}
    >
      <Group gap={4} wrap="nowrap">
        {children}
        <Text size="xs" c={active ? 'blue' : 'dimmed'} style={{ lineHeight: 1 }}>{icon}</Text>
      </Group>
    </Table.Th>
  )
}
