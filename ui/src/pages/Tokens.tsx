import { useState } from 'react'
import { fmtDateTime } from '../utils/date'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Stack, Title, Button, Table, Text, Group, Badge, ActionIcon, Modal,
  Select, NumberInput, Code, Alert, Loader, Center, TextInput,
} from '@mantine/core'
import { IconPlus, IconTrash, IconAlertCircle } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { listTokens, createToken, revokeToken } from '../api/misc'

export default function Tokens() {
  const qc = useQueryClient()
  const [modalOpen, setModalOpen] = useState(false)
  const [revokeId, setRevokeId] = useState<string | null>(null)
  const [createdToken, setCreatedToken] = useState<string | null>(null)

  const [formType, setFormType] = useState('single_host')
  const [formHostId, setFormHostId] = useState('')
  const [formLabel, setFormLabel] = useState('')
  const [formLabelVal, setFormLabelVal] = useState('')
  const [formTTL, setFormTTL] = useState(86400)
  const [formMaxUses, setFormMaxUses] = useState(1)

  const { data: tokens = [], isLoading } = useQuery({
    queryKey: ['tokens'],
    queryFn: listTokens,
  })

  const createMut = useMutation({
    mutationFn: () => createToken({
      type: formType,
      target_host_id: formHostId || undefined,
      label_template: formLabel ? { [formLabel]: formLabelVal } : undefined,
      ttl_seconds: formTTL,
      max_uses: formMaxUses,
    }),
    onSuccess: (t) => {
      qc.invalidateQueries({ queryKey: ['tokens'] })
      setModalOpen(false)
      if (t.token) setCreatedToken(t.token)
    },
    onError: () => notifications.show({ message: 'Create failed', color: 'red' }),
  })

  const revokeMut = useMutation({
    mutationFn: revokeToken,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['tokens'] })
      setRevokeId(null)
      notifications.show({ message: 'Token revoked', color: 'green' })
    },
  })

  const snippet = (tok: string) =>
    `curl -fsSL https://<CP_HOST>/install.sh | QF_TOKEN=${tok} sh`

  if (isLoading) return <Center h={200}><Loader /></Center>

  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Title order={2}>Bootstrap Tokens</Title>
        <Button leftSection={<IconPlus size={14} />} onClick={() => setModalOpen(true)}>
          New token
        </Button>
      </Group>

      <Table highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Type</Table.Th>
            <Table.Th>Labels</Table.Th>
            <Table.Th>Uses</Table.Th>
            <Table.Th>Expires</Table.Th>
            <Table.Th />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {tokens.map((t) => (
            <Table.Tr key={t.id}>
              <Table.Td><Badge size="sm">{t.type}</Badge></Table.Td>
              <Table.Td>
                {t.label_template && Object.entries(t.label_template).map(([k, v]) => (
                  <Badge key={k} size="xs" variant="outline">{k}={v}</Badge>
                ))}
              </Table.Td>
              <Table.Td>{t.uses_count}/{t.max_uses}</Table.Td>
              <Table.Td>{fmtDateTime(t.expires_at)}</Table.Td>
              <Table.Td>
                <ActionIcon color="red" variant="subtle" onClick={() => setRevokeId(t.id)}>
                  <IconTrash size={14} />
                </ActionIcon>
              </Table.Td>
            </Table.Tr>
          ))}
          {tokens.length === 0 && (
            <Table.Tr>
              <Table.Td colSpan={5}>
                <Text c="dimmed" ta="center" size="sm" py="md">No tokens</Text>
              </Table.Td>
            </Table.Tr>
          )}
        </Table.Tbody>
      </Table>

      <Modal opened={modalOpen} onClose={() => setModalOpen(false)} title="Create bootstrap token">
        <Stack gap="sm">
          <Select
            label="Type"
            data={['single_host', 'bulk']}
            value={formType}
            onChange={(v) => setFormType(v ?? 'single_host')}
          />
          {formType === 'single_host' && (
            <TextInput
              label="Target host ID (optional)"
              value={formHostId}
              onChange={(e) => setFormHostId(e.currentTarget.value)}
            />
          )}
          <Group>
            <TextInput
              label="Label key"
              value={formLabel}
              onChange={(e) => setFormLabel(e.currentTarget.value)}
              style={{ flex: 1 }}
            />
            <TextInput
              label="Label value"
              value={formLabelVal}
              onChange={(e) => setFormLabelVal(e.currentTarget.value)}
              style={{ flex: 1 }}
            />
          </Group>
          <NumberInput
            label="TTL (seconds)"
            value={formTTL}
            onChange={(v) => setFormTTL(Number(v))}
          />
          <NumberInput
            label="Max uses"
            value={formMaxUses}
            onChange={(v) => setFormMaxUses(Number(v))}
          />
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setModalOpen(false)}>Cancel</Button>
            <Button loading={createMut.isPending} onClick={() => createMut.mutate()}>Create</Button>
          </Group>
        </Stack>
      </Modal>

      <Modal
        opened={!!createdToken}
        onClose={() => setCreatedToken(null)}
        title="Token created"
        size="lg"
      >
        <Stack gap="md">
          <Alert icon={<IconAlertCircle size={16} />} color="yellow">
            Copy this token now. It will not be shown again.
          </Alert>
          <Code block>{createdToken}</Code>
          <Text fw={600}>Install snippet:</Text>
          <Code block>{snippet(createdToken ?? '')}</Code>
          <Button onClick={() => setCreatedToken(null)}>Close</Button>
        </Stack>
      </Modal>

      <Modal opened={!!revokeId} onClose={() => setRevokeId(null)} title="Revoke token" size="sm">
        <Stack gap="md">
          <Text>Revoke this token? Enrolled hosts are unaffected.</Text>
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setRevokeId(null)}>Cancel</Button>
            <Button color="red" loading={revokeMut.isPending} onClick={() => revokeId && revokeMut.mutate(revokeId)}>
              Revoke
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  )
}
