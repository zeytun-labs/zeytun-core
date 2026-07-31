# 🫒 zeytun-core

![Go Version](https://img.shields.io/github/go-mod/go-version/sadeqi-ah/zeytun-core?style=flat-square)
![License](https://img.shields.io/badge/License-GPL%203.0-blue.svg?style=flat-square)

**Application-aware network routing and traffic control engine for the Zeytun ecosystem.**

`zeytun-core` is an open-source networking engine focused on intelligent routing, proxy management, DNS handling, TUN/system integration, and runtime network control.

The project is designed as the foundation for the upcoming Zeytun desktop application, providing the low-level networking capabilities required for interactive routing, traffic visibility, and advanced network policy management.

Unlike traditional proxy cores that mainly focus on forwarding traffic, `zeytun-core` aims to provide deeper control over how applications communicate with the internet through policy-driven routing and live connection management.

> ⚠️ **Note:** `zeytun-core` is a specialized networking engine developed for the Zeytun ecosystem. It is not intended to be a standalone proxy platform.

---

## ✨ Zeytun Extensions

`zeytun-core` is based on [sing-box](https://github.com/SagerNet/sing-box) and extends it with Zeytun-specific functionality focused on interactive routing, traffic visibility, and desktop integration.

## 🛡 Interactive Routing

One of the core concepts of Zeytun is firewall-like interactive routing.

When a network connection does not match existing routing policies, the core can temporarily hold the connection and expose it to a controlling client for user decision-making.

Possible actions include:

- Block the connection
- Allow the connection directly
- Route the connection through a selected proxy
- Apply a custom routing policy

These decisions can be forwarded to higher-level components, which can manage rule persistence and lifecycle before applying them back to the core.

---

## 🔄 Live Routing Updates

`zeytun-core` supports runtime routing updates without requiring a restart.

The core provides mechanisms for external controllers to inject and update routing rules while the networking engine is running.

Capabilities include:

- Live rule injection
- Runtime routing table updates
- External control through APIs
- Applying routing decisions without restarting the core

Rule lifecycle management, persistence, and expiration policies are handled by higher-level components such as the Zeytun backend.

---

## 📊 Traffic Visibility and Event Streaming

The core provides live information required for network analysis and user-facing interfaces.

Available capabilities include:

- Connection events
- DNS activity
- Network statistics
- Connection metadata
- Process-related information
- Runtime lifecycle events

These capabilities are exposed through control interfaces designed for integration with desktop applications and other clients.

---

## ⚖ Advanced Load Balancing

Zeytun extends outbound management with additional balancing strategies.

Supported strategies include:

- `round-robin`
- `consistent-hashing`
- `sticky-sessions`
- `failover`
- `weighted`
- `least-connections`

---

## ⚡ Transport and Networking Extensions

Additional networking improvements developed for the Zeytun ecosystem include:

- Native XHTTP transport support
- Zeytun-specific routing improvements
- Extended runtime control capabilities

---

## 🔌 System Networking Integration

Current capabilities:

- TUN-based routing
- Rule-based routing
- Proxy outbound management
- DNS handling
- Runtime control APIs

Planned integrations:

- macOS Network Extensions
- Windows Filtering Platform
- Advanced process-level network interception

---

## 🔧 Configuration Compatibility

Where Zeytun-specific extensions are not required, configuration remains compatible with standard sing-box JSON configuration formats.

Existing sing-box configurations can be reused with minimal modifications.

---

## 🏗 Architecture

`zeytun-core` provides the networking foundation for future Zeytun clients:

```
┌──────────────────────────────────────┐
│          Zeytun Desktop              │
│       (Tauri + Svelte)               │
│                                      │
│  UI / User Interaction / Analytics   │
└──────────────────┬───────────────────┘
                   │
                   │
┌──────────────────▼───────────────────┐
│          Zeytun Backend              │
│                                      │
│  Policy Management                   │
│  Rule Lifecycle                      │
│  Persistence                         │
│  User Decisions                      │
└──────────────────┬───────────────────┘
                   │
                   │ gRPC
                   │
┌──────────────────▼───────────────────┐
│            zeytun-core               │
│                                      │
│  Routing Engine                      │
│  Rule Evaluation                     │
│  DNS                                 │
│  TUN                                 │
│  Proxy Outbounds                     │
│  Live Rule Injection                 │
└──────────────────┬───────────────────┘
                   │
                   ▼
          ┌────────────────┐
          │ Operating      │
          │ System         │
          └────────────────┘
```
---

## ⚖ Relationship to sing-box

`zeytun-core` is a fork and derivative work of [sing-box](https://github.com/SagerNet/sing-box) by SagerNet / nekohasekai.

The project builds upon sing-box's networking foundation and adds Zeytun-specific capabilities, including:

- Interactive routing workflows
- Live routing rule injection
- Desktop application integration interfaces
- Additional routing and management features

This project is not affiliated with, endorsed by, or an official product of the sing-box project.

Upstream copyright, attribution, and GPL-3.0 licensing requirements are preserved.

---

## 📄 License

`zeytun-core` is a derivative work based on [sing-box](https://github.com/SagerNet/sing-box).

The majority of the codebase originates from sing-box and remains subject to the original upstream copyright and GPL-3.0 license terms.

Zeytun-specific modifications and additions are distributed under the same GPL-3.0 license.

All upstream copyright notices are preserved.

See [`LICENSE`](./LICENSE) for the full license text and licensing information.