# Networking Basics

*Middle School Level*

---

## How Computers Talk to Each Other

Every computer on the internet has an address, just like every house on a
street has a number. When one computer wants to send information to another,
it packages that information into **packets** and sends them across the
network using that address.

This chapter explains the building blocks of computer networking: the
concepts you need to understand before Unheaded's approach will make sense.

---

## IP Addresses

An **IP address** (Internet Protocol address) uniquely identifies a computer
on a network. There are two versions:

### IPv4

The original format. Four numbers from 0 to 255, separated by dots:

```
192.168.1.100
```

IPv4 supports roughly 4.3 billion addresses. That sounds like a lot, but we
ran out years ago. There are more devices on the internet than IPv4 addresses.

### IPv6

The newer format. Eight groups of hexadecimal digits separated by colons:

```
fd00:3f:75:1::1
```

IPv6 supports 340 undecillion addresses (that is a 34 followed by 37 zeros).
We will never run out.

**Unheaded uses IPv6 internally.** This is not just about having more
addresses -- IPv6 has a feature called **extension headers** that lets you
attach extra information to packets. Unheaded uses this feature to carry the
Monad tracking data.

### Private vs. Public

Some address ranges are reserved for internal networks:

```
Private IPv4: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
Private IPv6: fd00::/8 (Unique Local Addresses)
```

Traffic on private addresses stays inside your network. Unheaded operates on
private addresses within its managed domain.

---

## Ports

If an IP address is like a street address, a **port** is like an apartment
number. A single computer (one IP address) can run many services, and each
service listens on a different port.

Ports are numbers from 0 to 65535.

```
          Server: 10.10.10.20
          +-----------------------+
          |                       |
  Port 80 | Web Server (HTTP)    |
          |                       |
 Port 443 | Web Server (HTTPS)   |
          |                       |
Port 5432 | Database (PostgreSQL)|
          |                       |
Port 8080 | API Server           |
          |                       |
          +-----------------------+
```

When your browser connects to a website, it connects to a specific IP address
on a specific port. For example, `https://example.com` connects to the
server's IP address on port 443 (the standard port for HTTPS).

### Well-Known Ports

| Port | Service | What It Does |
|------|---------|-------------|
| 22 | SSH | Remote terminal access |
| 53 | DNS | Translates domain names to IP addresses |
| 80 | HTTP | Unencrypted web traffic |
| 443 | HTTPS | Encrypted web traffic |
| 5432 | PostgreSQL | Database queries |

### Unheaded's Port Range

Unheaded uses ports 16666 through 26666 (called "the Doom Range") to avoid
conflicting with common services:

```
16666-16999  Infrastructure (trace-collector, bridges)
17000-17999  Control Plane (unheaded-daemon)
18000-18099  Wotan message bus
19000-19999  Core services (timeguru, architect, captain)
20000-20999  Applications (dashboard, kanban)
21000-21443  Gateway (HTTP/HTTPS)
26000-26666  Reserved for user applications
```

---

## TCP and UDP

These are the two main **transport protocols** -- the rules for how packets
are actually sent between computers.

### TCP (Transmission Control Protocol)

TCP is like a phone call. Before you start talking, you dial and wait for the
other person to pick up. Then you have a conversation where both sides
acknowledge what they heard.

```
Client                    Server
  |                         |
  |--- SYN (Hello!) ------->|  Step 1: Client says hello
  |                         |
  |<-- SYN+ACK (Hi back!) --|  Step 2: Server responds
  |                         |
  |--- ACK (Great!) ------->|  Step 3: Client confirms
  |                         |
  |===== Connection Open ===|  Now they can exchange data
  |                         |
  |--- Data: "GET /page" -->|  Client sends a request
  |                         |
  |<-- Data: "<html>..." ---|  Server sends the page
  |                         |
  |--- FIN (Goodbye) ------>|  Client closes connection
  |<-- ACK (Bye) -----------|  Server acknowledges
```

This three-step opening is called the **TCP three-way handshake** (SYN,
SYN+ACK, ACK). It makes sure both sides are ready before data starts flowing.

TCP guarantees:
- Data arrives in order
- Lost packets are re-sent
- Both sides know when the connection is over

### UDP (User Datagram Protocol)

UDP is like shouting across a room. You send your message and hope the other
person hears it. There is no handshake, no confirmation, no re-sending.

```
Client                    Server
  |                         |
  |--- Data: "Hello!" ----->|  Sent. Maybe it arrives. Maybe not.
  |                         |
  |--- Data: "World!" ----->|  Another message. No waiting.
  |                         |
```

UDP is faster because it skips all the handshaking and tracking. It is used
for things where speed matters more than perfection: video calls, online
games, DNS lookups.

### Which One Does Unheaded Use?

Both. Unheaded's internal communication uses **gRPC** (which runs on TCP) for
reliable service-to-service messaging. The eBPF programs track both TCP and
UDP flows.

---

## The 5-Tuple

Every network connection can be uniquely identified by five pieces of
information, called the **5-tuple**:

```
+---------------------------------------------------+
|                    5-TUPLE                         |
|                                                   |
|  1. Source IP Address      192.168.1.5            |
|  2. Destination IP Address 10.10.10.20            |
|  3. Source Port            54321                  |
|  4. Destination Port       443                    |
|  5. Protocol               TCP (6)                |
|                                                   |
+---------------------------------------------------+
```

If any one of these five values is different, it is a different connection.
This is how Unheaded's flow tracker identifies and distinguishes connections.

---

## Clients and Servers

A **client** is a program that asks for something. A **server** is a program
that provides it.

```
    CLIENT                         SERVER
  +-----------+               +-------------+
  | Browser   | --- request ->| Web Server  |
  |           | <- response --| (nginx)     |
  +-----------+               +-------------+
     You                        The website
```

The same computer can be both a client and a server. When your web browser
asks the dashboard for data, it is a client. When the dashboard asks Wotan
for messages, the dashboard is a client and Wotan is the server.

In Unheaded's architecture:

```
Browser (client)
    |
    v
Gateway (server to browser, client to services)
    |
    +---> Dashboard Backend (server to gateway, client to Wotan)
    |         |
    |         v
    |     Wotan (server to all services)
    |         ^
    |         |
    +---> Timeguru (server to gateway, client to Wotan)
    |
    +---> Captain, Architect, Micromanager... (same pattern)
```

---

## Routers and Switches

### Switches

A **switch** connects computers on the same local network. It reads the
destination address of each packet and sends it to the correct port. Think
of it as a hallway with labeled doors -- the switch directs each packet to
the right door.

```
        +--------+
  PC A--| Switch |--PC B
  PC C--|        |--PC D
        +--------+
```

### Routers

A **router** connects different networks together. When a packet needs to
leave the local network, the switch sends it to the router. The router
decides which network to forward it to next.

```
  Local Network A          Local Network B
  +--------+          +--------+          +--------+
  | Switch |---[Router A]---[Router B]---| Switch |
  +--------+                              +--------+
  10.0.1.x                                10.0.2.x
```

Every router maintains a **routing table**: a list of "if the destination is
in network X, send it through interface Y." The packet hops from router to
router until it reaches the destination network.

---

## Firewalls

A **firewall** is a filter between networks. It examines every packet and
decides whether to let it through or block it.

```
  Internet                    Your Network
            +----------+
  -------->| FIREWALL  |-------->  Allowed
            |          |
  -------->| Rules:    |----X     Blocked
            | Allow 443|
            | Allow 80 |
            | Deny *   |
            +----------+
```

Firewalls use rules based on IP addresses, ports, and protocols:

- "Allow TCP traffic to port 443 from anywhere" (let HTTPS through)
- "Block all traffic from 198.51.100.0/24" (block a bad network)
- "Deny everything not explicitly allowed" (default deny)

**Unheaded uses default-deny firewalls.** Every container starts with all
ports blocked. Only the specific ports a service needs are opened.

---

## DNS: The Phone Book

**DNS** (Domain Name System) translates human-readable names into IP
addresses:

```
You type: google.com
DNS says: 142.250.80.4
Browser connects to: 142.250.80.4:443
```

Without DNS, you would have to memorize the IP address of every website.

---

## Monitoring

**Monitoring** means watching a system to know if it is healthy. There are
several kinds:

### Health Checks

A simple question: "Are you alive?" The monitoring system sends a request to
each service and checks if it gets a response.

```
Monitor: GET /health --> Service A
Service A: 200 OK {"status": "healthy"}

Monitor: GET /health --> Service B
Service B: (no response)  <-- PROBLEM!
```

### Metrics

Numbers that describe how the system is performing:

- CPU usage: 45%
- Memory usage: 2.1 GB
- Requests per second: 1,247
- Error rate: 0.02%
- Response time (p99): 42ms

### Logging

Text records of what happened:

```
2026-02-24 15:00:01 INFO  timeguru: request completed, status=200, duration=12ms
2026-02-24 15:00:02 WARN  wotan: ring buffer 80% full
2026-02-24 15:00:03 ERROR captain: connection to architect timed out after 5s
```

### Tracing

Following a single request through multiple services:

```
Request #abc123:
  --> Gateway        (2ms)
    --> Dashboard    (5ms)
      --> Wotan      (1ms)
      <-- Wotan      (1ms response)
    <-- Dashboard    (3ms processing)
  <-- Gateway        (1ms overhead)
  Total: 13ms
```

**Unheaded does all four.** Health checks, metrics, logging, and tracing are
built into every service. The eBPF layer adds packet-level tracing that
operates below the application, in the kernel itself.

---

## Putting It Together

Here is how a typical request flows through a network with all of these
concepts in play:

```
1. Browser resolves "dashboard.example.com" via DNS
       |
2. Browser opens TCP connection to 10.10.10.100:443 (3-way handshake)
       |
3. Firewall allows port 443, passes the packet
       |
4. Gateway receives the request, routes to dashboard-backend
       |
5. Dashboard-backend queries Wotan on port 18001 (gRPC)
       |
6. Wotan returns data, dashboard-backend formats response
       |
7. Response travels back through gateway to browser
       |
8. Each step is logged, measured, and traced
```

Every one of those steps involves packets with IP addresses, port numbers,
and either TCP or UDP. Every one of those packets is monitored. And in an
Unheaded network, every one of those packets carries a Monad that tracks its
entire journey at the kernel level.

---

*Next: [What Unheaded Does](what-unheaded-does.md) -- The three pillars of
networking, observability, and security.*
