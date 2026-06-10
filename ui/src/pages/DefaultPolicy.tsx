import { useState, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Stack, Title, Select, Button, Group, Text, Card, Alert, Modal, Loader, Center,
} from '@mantine/core'
import { IconAlertTriangle } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { getDefaultPolicy, updateDefaultPolicy } from '../api/misc'

const ACTIONS = ['allow', 'deny']

export default function DefaultPolicy() {
  const qc = useQueryClient()
  const { data: dp, isLoading } = useQuery({
    queryKey: ['default-policy'],
    queryFn: getDefaultPolicy,
  })

  const [ingress, setIngress] = useState('allow')
  const [egress, setEgress] = useState('allow')
  const [warnOpen, setWarnOpen] = useState(false)

  useEffect(() => {
    if (dp) {
      setIngress(dp.default_ingress_action)
      setEgress(dp.default_egress_action)
    }
  }, [dp])

  const saveMut = useMutation({
    mutationFn: () => updateDefaultPolicy({
      default_ingress_action: ingress,
      default_egress_action: egress,
    }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['default-policy'] })
      setWarnOpen(false)
      notifications.show({ message: 'Default policy saved', color: 'green' })
    },
  })

  function handleSave() {
    const prevIngress = dp?.default_ingress_action ?? 'allow'
    const prevEgress = dp?.default_egress_action ?? 'allow'
    const changingToDeny =
      (prevIngress !== 'deny' && ingress === 'deny') ||
      (prevEgress !== 'deny' && egress === 'deny')
    if (changingToDeny) {
      setWarnOpen(true)
    } else {
      saveMut.mutate()
    }
  }

  if (isLoading) return <Center h={200}><Loader /></Center>

  return (
    <Stack gap="md">
      <Title order={2}>Default Policy</Title>
      <Text c="dimmed" size="sm">
        Applied when no explicit rule matches. Changing to Deny blocks all unmatched traffic.
      </Text>

      <Card withBorder maw={400}>
        <Stack gap="md">
          <Select
            label="Default ingress action"
            data={ACTIONS}
            value={ingress}
            onChange={(v) => setIngress(v ?? 'allow')}
          />
          <Select
            label="Default egress action"
            data={ACTIONS}
            value={egress}
            onChange={(v) => setEgress(v ?? 'allow')}
          />
          <Button onClick={handleSave} loading={saveMut.isPending}>
            Save
          </Button>
        </Stack>
      </Card>

      <Modal
        opened={warnOpen}
        onClose={() => setWarnOpen(false)}
        title="Confirm dangerous change"
        size="sm"
      >
        <Stack gap="md">
          <Alert icon={<IconAlertTriangle size={16} />} color="red">
            Changing default action to <strong>Deny</strong> will block all traffic not matched by an explicit rule.
            This may cut off access to hosts.
          </Alert>
          <Group justify="flex-end">
            <Button variant="subtle" onClick={() => setWarnOpen(false)}>Cancel</Button>
            <Button color="red" loading={saveMut.isPending} onClick={() => saveMut.mutate()}>
              Apply anyway
            </Button>
          </Group>
        </Stack>
      </Modal>
    </Stack>
  )
}
