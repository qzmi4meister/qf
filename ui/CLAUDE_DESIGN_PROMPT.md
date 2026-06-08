# Claude Design brief — qf control plane Web UI

## What I want from you

Design a cohesive, modern visual design system and key-screen layouts for the **qf control plane Web UI**. The app already exists and is fully functional; I am **not** asking for new features or a rewrite. I want a **design pass**: a refined design language (color, typography, spacing, density, components, states) and high-fidelity mockups of the most important screens, that I can then translate back into the existing Mantine codebase.

Treat this as redesigning the front-end of a serious infrastructure/security product — think Tailscale admin, Cloudflare dashboard, Grafana, Cilium Hubble UI, Vault UI. Clean, dense, "operator-grade", trustworthy. Not a flashy marketing site.

## Product context

**qf** is a centrally-managed, **eBPF-based host firewall**. A lightweight agent runs on each Linux host and enforces firewall rules pushed from a central control plane over mTLS gRPC. Rules are expressed as **policies with label selectors**; the control plane compiles them to BPF maps and pushes bundles to agents.

This UI is the **control plane admin console** — the single pane of glass where an operator (SRE / platform / security engineer) manages fleets of hosts, authors firewall policies, and investigates traffic.

Core domain concepts the design must represent well:
- **Hosts** — Linux machines running the agent. Have a status (`active` / `offline` / `enrolling` / `error`), labels (key=value), agent version, kernel version, generation number, last-heartbeat time.
- **Policies** — named, prioritized rule sets with a **label selector** that targets hosts. Each policy contains ordered **rules**.
- **Rules** — direction (`ingress`/`egress`/`both`), action (`allow`/`deny`/`log`), priority, match conditions (protocol, src/dst CIDR, src/dst ports), log/silent flags.
- **Object groups** — reusable IP sets / port sets / host sets referenced across rules.
- **Default policy** — global default ingress/egress action.
- **Events (log events)** — per-rule packet hits (action, protocol, src/dst ip:port, ct state).
- **Flows** — conntrack-based per-connection telemetry (bytes/packets each direction, final state).
- **Audit log** — every control-plane mutation with actor + before/after.
- **Tokens** — agent enrollment tokens + API tokens.
- **Users** — accounts with roles, optional OIDC/SSO.

## Current tech stack (must stay compatible)

- **React 19 + TypeScript + Vite**
- **Mantine 7** as the component library (`@mantine/core`, `@mantine/charts`, `@mantine/hooks`, `@mantine/modals`, `@mantine/notifications`)
- **@tabler/icons-react** for icons
- **@tanstack/react-table** + **react-virtual** for data tables
- **recharts / @mantine/charts** for charts
- **react-router-dom** (SPA, mounted under `/app`)
- Layout uses Mantine **AppShell** (top header 56px + left navbar 220px)

**Constraint:** the output must be realistically implementable in **Mantine 7** with minimal custom CSS. Prefer restyling/theming Mantine primitives (Card, Table, Badge, Tabs, NavLink, Paper, Modal) over inventing components that Mantine can't express. Use Mantine theme tokens (CSS variables, radius/spacing scale, color shades) where possible. Where you propose something beyond stock Mantine, call it out explicitly so I know it needs custom work.

## Current design (what exists today — the baseline you're improving)

- **Dark-first**, with a working light/dark toggle (`defaultColorScheme="auto"`). Keep both themes; dark is primary.
- Current dark palette is a hand-tuned **slate** ramp (slate-950 body → slate-800 cards), primary color **indigo**, with semantic accents: green=active/allow, red=offline-error/deny, yellow=enrolling/changed, blue=info/log, violet=policies.
- Layout: fixed left nav with these sections (in order): **Dashboard, Hosts, Policies, Object Groups, Default Policy, Events, Flows, Audit Log, Tokens, Users**. Header shows the `qf` shield-lock logo wordmark, tenant name, theme toggle, and a user avatar menu. Nav shows an offline-host count badge on "Hosts" and an app version at the bottom.
- It currently looks generic/default-Mantine. I want it to feel **designed and intentional** — a real product identity — while staying clean and dense.

## Brand / tone

- Name is lowercase **`qf`**. Current logo is a Tabler `IconShieldLock`. You may propose a simple, distinctive wordmark/logomark (firewall / shield / packet / eBPF motif), kept minimal and monochrome-friendly. Nothing skeuomorphic.
- Tone: precise, calm, technical, secure. Monospaced type for IPs/ports/IDs/hashes is welcome.
- Density: **information-dense but legible**. Operators stare at this for hours. Comfortable-compact tables, clear status semantics, fast scanning.

## Deliverables

1. **Design system / style guide**
   - Color: refined dark palette + matching light palette, defined as a Mantine-compatible shade ramp; semantic color mapping for the statuses/actions above (active/offline/enrolling/error, allow/deny/log).
   - Typography scale (UI font + monospace for technical values), heading/label/body sizes.
   - Spacing, radius, border, elevation conventions; card/table/badge styling.
   - Iconography guidance (Tabler set).
   - A reusable **status badge** and **action badge** treatment, since these appear everywhere.

2. **Key screen mockups** (high fidelity, dark theme primary; show light variant for at least the Dashboard):
   - **Dashboard** — fleet overview: KPI stat cards (total hosts + status breakdown, policy count), host-status chart, recent audit events table.
   - **Hosts list** — dense sortable/filterable table (hostname, status badge, agent version, generation, last seen, labels as chips, row actions) with search + status filter.
   - **Host detail** — tabbed view (Overview / Effective ruleset / Events / Flows / Counters), host metadata header with status + flow-events toggle, label editor.
   - **Policy detail / editor** — the most complex screen: policy metadata, label-selector editor, an ordered **rules table**, an inline **rule editor** (direction/action/priority/match conditions/log+silent), plus "Preview impact" and "Assign to hosts" modals and a version-history tab. Make rule authoring feel clear and safe.
   - **Flows explorer** and **Events** — high-volume telemetry tables (virtualized), with filters; show how dense time-series rows should look.
   - **Login** — minimal centered card with email/password + optional SSO button.

3. For each screen: show realistic populated data **and** the relevant **empty / loading / error** states, since those matter a lot for an ops tool.

## Explicit non-goals

- No new product features or backend changes.
- No marketing/landing pages.
- Don't drop Mantine or propose a different framework.
- Don't over-decorate — restraint over flourish.

## How I'll use the output

I'll take your design system + screen mockups and re-theme the existing Mantine app (`createTheme` tokens, component styling, layout tweaks) to match. So please keep recommendations concrete and mapped to Mantine where you can, and flag anything that needs custom CSS or a custom component.
