// Placeholder hosts until the SQLite host store and ~/.ssh/config import land.
export type HostStatus = 'online' | 'degraded' | 'offline'

export interface Host {
  name: string
  target: string
  status: HostStatus
  latency?: string
}

export interface HostGroup {
  name: string
  hosts: Host[]
}

export const sampleGroups: HostGroup[] = [
  {
    name: 'PostgreSQL · prod',
    hosts: [
      { name: 'pg-primary', target: 'postgres@10.0.0.11', status: 'online', latency: '3ms' },
      { name: 'pg-replica-1', target: 'postgres@10.0.0.12', status: 'online', latency: '3ms' },
      { name: 'haproxy-01', target: 'admin@10.0.0.5', status: 'degraded', latency: '88ms' },
    ],
  },
  {
    name: 'Proxmox',
    hosts: [
      { name: 'pve-node-01', target: 'root@10.0.1.21', status: 'online', latency: '5ms' },
      { name: 'pve-old-01', target: 'root@10.0.1.9', status: 'offline' },
    ],
  },
]
