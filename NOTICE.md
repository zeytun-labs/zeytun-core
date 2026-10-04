# NOTICE

zeytun-core is a derivative work of sing-box.

## Upstream

sing-box — the universal proxy platform
https://github.com/SagerNet/sing-box

Copyright (C) 2022 by nekohasekai <contact-sagernet@sekai.icu>
and sing-box contributors.

Licensed under the GNU General Public License v3.0. The upstream license
also states that no derivative work may use the name "sing-box" or imply
association with that application without prior consent. This project
complies: it is distributed as "zeytun-core" and makes no such claim.

## Modifications by Zeytun

Copyright (C) 2026 AmirHossein Sadeghi, also under GPLv3.

- `route/ask.go` — connection-ask checkpoint
- `experimental/clashapi/connection_ask.go` — clash API surface for it
- live rule updates without daemon restart
- coreevent gRPC event stream
- remote ruleset soft-fail
- balancer outbound
