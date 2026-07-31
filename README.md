# 🫒 zeytun-core

![Go Version](https://img.shields.io/github/go-mod/go-version/sadeqi-ah/zeytun-core?style=flat-square)
![License](https://img.shields.io/badge/License-GPL%203.0%20Modified-blue.svg?style=flat-square)

**Network and routing core for the [Zeytun](https://github.com/zeytun-labs) desktop app.**

`zeytun-core` handles proxy outbounds, policy groups, DNS, rule-based routing, TUN/system integration, and live control surfaces (gRPC / Clash API) used by the Zeytun client.

> ⚠️ **Note:** This is **not** a general-purpose standalone proxy platform product. It is a highly specialized engine tailored specifically for the Zeytun application ecosystem.

---

## ✨ Notable Extensions (vs Upstream)

This core includes several Zeytun-oriented modifications and custom features built on top of the fork base:

* **⚖️ Advanced Load Balancing:** Added dedicated balancer outbounds supporting multiple strategies (`round-robin`, `consistent-hashing`, `sticky-sessions`, `failover`, `weighted`, `least-connections`).
* **⚡ Next-Gen Transports:** Native support for the **XHTTP** transport protocol.
* **🛡️ Interactive & Live Routing:**
  * **Connection-Ask:** Intercepts unmatched TCP connections for interactive routing decisions (firewall-like behavior).
  * **Live Rule Overlay:** Support for injecting temporary and permanent routing rules on the fly.
* **🔄 Resilient Rule-sets:** Implemented remote rule-set soft-fail and background re-fetching on startup (prevents hard crashes on DNS failures).
* **🔌 Deep App Integration:** CoreEvent gRPC stream and lifecycle hooks designed specifically for seamless communication with the Zeytun desktop UI.

*(Configuration remains fully compatible with standard sing-box JSON format wherever not explicitly extended.)*

---

## ⚖️ Relationship to sing-box & License

`zeytun-core` is a **derivative** of [sing-box](https://github.com/SagerNet/sing-box) (SagerNet / nekohasekai).

It is **not** affiliated with, endorsed by, or an official product of SagerNet or the sing-box project. Per upstream terms, this work does **not** use the sing-box name as its product identity and does **not** claim association with that application.

* **Upstream project and docs:** [sing-box.sagernet.org](https://sing-box.sagernet.org)


### License Terms

```text
Copyright (C) 2022 by nekohasekai <contact-sagernet@sekai.icu>

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see [http://www.gnu.org/licenses/](http://www.gnu.org/licenses/).

In addition, no derivative work may use the name or imply association
with this application without prior consent.

```

**Full text:** [`LICENSE`](./LICENSE). Upstream copyright is strictly retained. Modifications specifically made for Zeytun are distributed under the exact same **GPL-3.0** terms.