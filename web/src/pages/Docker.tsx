import { useCallback, useEffect, useState } from 'react'
import {
  Alert, Button, Drawer, Popconfirm, Space, Tabs, Table, Tag, Typography, message,
} from 'antd'
import {
  CaretRightOutlined, DeleteOutlined, FileTextOutlined, InfoCircleOutlined,
  PauseOutlined, ReloadOutlined, SearchOutlined,
} from '@ant-design/icons'
import { api, ContainerRow, ImageRow } from '../services/api'

type Volume = { Name?: string; Driver?: string; Mountpoint?: string; CreatedAt?: string }
type Network = { Name?: string; Id?: string; Driver?: string; Scope?: string }

function formatSize(bytes: number): string {
  if (bytes <= 0) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let i = 0
  let v = bytes
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++ }
  return `${v.toFixed(1)} ${units[i]}`
}

export default function Docker() {
  const [tab, setTab] = useState('containers')
  const [containers, setContainers] = useState<ContainerRow[]>([])
  const [images, setImages] = useState<ImageRow[]>([])
  const [volumes, setVolumes] = useState<Volume[]>([])
  const [networks, setNetworks] = useState<Network[]>([])
  const [loading, setLoading] = useState(false)
  const [dockerError, setDockerError] = useState<string | null>(null)
  const [logsFor, setLogsFor] = useState<ContainerRow | null>(null)
  const [logs, setLogs] = useState('')
  const [inspectFor, setInspectFor] = useState<ContainerRow | null>(null)
  const [inspect, setInspect] = useState<string>('')
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    setDockerError(null)
    try {
      if (tab === 'containers') setContainers((await api.get<ContainerRow[]>('/api/docker/containers?all=1')) ?? [])
      else if (tab === 'images') setImages((await api.get<ImageRow[]>('/api/docker/images')) ?? [])
      else if (tab === 'volumes') setVolumes((await api.get<Volume[]>('/api/docker/volumes')) ?? [])
      else if (tab === 'networks') setNetworks((await api.get<Network[]>('/api/docker/networks')) ?? [])
    } catch (e: any) {
      setDockerError(e.message)
    } finally {
      setLoading(false)
    }
  }, [tab])

  useEffect(() => { load() }, [load])

  const action = async (id: string, act: string) => {
    setBusy(true)
    try {
      await api.post(`/api/docker/containers/${id}/${act}`, {})
      message.success(`${act} 完成`)
      load()
    } catch (e: any) { message.error(e.message) } finally { setBusy(false) }
  }

  const removeContainer = async (row: ContainerRow) => {
    setBusy(true)
    try {
      await api.del(`/api/docker/containers/${row.id}?force=1`)
      message.success('已删除')
      load()
    } catch (e: any) { message.error(e.message) } finally { setBusy(false) }
  }

  const showLogs = async (row: ContainerRow) => {
    setLogsFor(row)
    setLogs('加载中…')
    try {
      const res = await api.get<{ logs: string }>(`/api/docker/containers/${row.id}/logs?tail=300`)
      setLogs(res.logs || '(无日志)')
    } catch (e: any) {
      setLogs(`加载失败: ${e.message}`)
    }
  }

  const showInspect = async (row: ContainerRow) => {
    setInspectFor(row)
    setInspect('加载中…')
    try {
      const res = await api.get<any>(`/api/docker/containers/${row.id}/inspect`)
      setInspect(JSON.stringify(res, null, 2))
    } catch (e: any) {
      setInspect(`加载失败: ${e.message}`)
    }
  }

  if (dockerError) {
    return (
      <Alert
        type="error"
        message="Docker Engine 不可达"
        description={dockerError}
        action={<Button onClick={load} icon={<ReloadOutlined />}>重试</Button>}
      />
    )
  }

  const stateColor: Record<string, string> = {
    running: 'green', exited: 'red', paused: 'orange', created: 'blue', restarting: 'cyan',
  }

  return (
    <div>
      <Tabs
        activeKey={tab}
        onChange={setTab}
        tabBarExtraContent={
          <Button icon={<ReloadOutlined />} onClick={load} loading={loading}>刷新</Button>
        }
        items={[
          {
            key: 'containers',
            label: '容器',
            children: (
              <Table<ContainerRow>
                rowKey="id"
                size="small"
                loading={loading}
                dataSource={containers}
                columns={[
                  { title: '名称', dataIndex: 'name' },
                  { title: '镜像', dataIndex: 'image', ellipsis: true },
                  {
                    title: '状态', dataIndex: 'state',
                    render: (v, r) => <Tag color={stateColor[v] ?? 'default'}>{r.status}</Tag>,
                  },
                  {
                    title: '操作', key: 'actions', width: 320,
                    render: (_, row) => (
                      <Space>
                        <Button size="small" icon={<CaretRightOutlined />} loading={busy}
                          disabled={row.state === 'running'} onClick={() => action(row.id, 'start')}>启动</Button>
                        <Button size="small" icon={<PauseOutlined />} loading={busy}
                          disabled={row.state !== 'running'} onClick={() => action(row.id, 'stop')}>停止</Button>
                        <Button size="small" icon={<ReloadOutlined />} loading={busy}
                          onClick={() => action(row.id, 'restart')}>重启</Button>
                        <Button size="small" icon={<FileTextOutlined />} onClick={() => showLogs(row)}>日志</Button>
                        <Button size="small" icon={<InfoCircleOutlined />} onClick={() => showInspect(row)}>详情</Button>
                        <Popconfirm title={`删除容器 ${row.name}？`} onConfirm={() => removeContainer(row)}>
                          <Button size="small" danger icon={<DeleteOutlined />}>删除</Button>
                        </Popconfirm>
                      </Space>
                    ),
                  },
                ]}
              />
            ),
          },
          {
            key: 'images',
            label: '镜像',
            children: (
              <Table<ImageRow>
                rowKey="id"
                size="small"
                loading={loading}
                dataSource={images}
                columns={[
                  { title: 'ID', dataIndex: 'id' },
                  { title: '标签', dataIndex: 'tag', ellipsis: true },
                  { title: '大小', dataIndex: 'size', render: (v) => formatSize(v) },
                  {
                    title: '创建时间', dataIndex: 'created',
                    render: (v) => new Date(v * 1000).toLocaleString(),
                  },
                ]}
              />
            ),
          },
          {
            key: 'volumes',
            label: '卷',
            children: (
              <Table<Volume>
                rowKey={(v) => v.Name ?? ''}
                size="small"
                loading={loading}
                dataSource={volumes}
                columns={[
                  { title: '名称', dataIndex: 'Name', ellipsis: true },
                  { title: '驱动', dataIndex: 'Driver' },
                  { title: '挂载点', dataIndex: 'Mountpoint', ellipsis: true },
                  { title: '创建时间', dataIndex: 'CreatedAt', ellipsis: true },
                ]}
              />
            ),
          },
          {
            key: 'networks',
            label: '网络',
            children: (
              <Table<Network>
                rowKey={(v) => v.Id ?? v.Name ?? ''}
                size="small"
                loading={loading}
                dataSource={networks}
                columns={[
                  { title: '名称', dataIndex: 'Name' },
                  { title: 'ID', dataIndex: 'Id', render: (v) => (v ?? '').slice(0, 12) },
                  { title: '驱动', dataIndex: 'Driver' },
                  { title: '范围', dataIndex: 'Scope' },
                ]}
              />
            ),
          },
        ]}
      />

      <Drawer
        title={`日志：${logsFor?.name ?? ''}`}
        open={!!logsFor}
        onClose={() => setLogsFor(null)}
        width="70%"
      >
        <pre style={{
          background: '#1e1e1e', color: '#d4d4d4', padding: 12, borderRadius: 6,
          overflow: 'auto', fontSize: 12, maxHeight: '100%',
        }}>{logs}</pre>
      </Drawer>

      <Drawer
        title={`Inspect：${inspectFor?.name ?? ''}`}
        open={!!inspectFor}
        onClose={() => setInspectFor(null)}
        width="60%"
      >
        <pre style={{
          background: '#1e1e1e', color: '#d4d4d4', padding: 12, borderRadius: 6,
          overflow: 'auto', fontSize: 12, maxHeight: '100%',
        }}>{inspect}</pre>
      </Drawer>
    </div>
  )
}
