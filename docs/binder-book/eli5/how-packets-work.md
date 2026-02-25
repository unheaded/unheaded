# How Packets Work

*Explain Like I'm 5*

---

## Digital Envelopes

Remember how we talked about computers sending tiny digital envelopes? Let us
look inside one.

When you type "google.com" into your browser, your computer needs to send a
message to Google's computer. But it cannot send the whole message at once.
That would be like trying to mail a whole book in one envelope. Instead, it
breaks the message into small pieces and puts each piece in its own envelope.

Each envelope is a **packet**.

---

## What Is on the Outside

Just like a real envelope, a packet has information written on the outside.
This information is called the **header**. It tells the network how to deliver
the packet.

Here is what is on the outside of every packet:

```
+------------------------------------------+
|         THE OUTSIDE (HEADER)             |
|                                          |
|  From: 192.168.1.5  (your computer)     |
|  To:   142.250.80.4 (Google's computer) |
|  Type: TCP  (reliable delivery)          |
|  Port: 443  (the "mailbox slot")         |
|  Size: 1500 bytes                        |
|                                          |
+------------------------------------------+
```

Let us explain each part:

### The "From" Address (Source IP)

This is your computer's address on the network. Just like your home address
tells the postal service where you live, your IP address tells the network
where your computer is.

An IP address looks like four numbers separated by dots: `192.168.1.5`. Each
number is between 0 and 255.

There is also a newer kind of address called IPv6 that looks like groups of
letters and numbers separated by colons: `fd00:3f:75:1::1`. This is like the
difference between a 5-digit zip code and a 9-digit zip code -- more digits
means more possible addresses.

### The "To" Address (Destination IP)

This is the address of the computer you are sending to. The network reads
this address to figure out which direction to send the packet.

### The Type (Protocol)

There are two main ways to send packets:

- **TCP** (Transmission Control Protocol): Like registered mail. You get a
  confirmation that it was delivered. If it gets lost, it gets re-sent. Slower
  but reliable.

- **UDP** (User Datagram Protocol): Like dropping a postcard in a mailbox. You
  hope it arrives, but you do not get a confirmation. Faster but less reliable.

Web pages use TCP (because you want every word to arrive). Video calls
sometimes use UDP (because a tiny glitch is better than a long delay).

### The Port (Mailbox Slot)

Your computer runs many programs at once: a web browser, an email program, a
music player. The port number tells the computer which program should receive
this packet.

Think of it like an apartment building. The IP address gets you to the
building. The port number tells you which apartment to knock on.

Common ports:
- 80: Web pages (HTTP)
- 443: Secure web pages (HTTPS)
- 25: Email
- 53: Looking up website addresses (DNS)

### The Size

How much data is inside this particular envelope. Packets are usually between
40 and 1500 bytes.

---

## What Is on the Inside

The inside of the packet is called the **payload**. This is the actual
message -- the piece of the web page, the chunk of video, the text of an
email.

```
+------------------------------------------+
|         THE INSIDE (PAYLOAD)             |
|                                          |
|  <html>                                  |
|    <head><title>Google</title></head>    |
|    <body>...                             |
|                                          |
+------------------------------------------+
```

The network does not care what is inside the payload. Just like the post
office does not care whether you are sending a birthday card or a tax return.
It just reads the address on the outside and delivers it.

---

## How It Gets There

A packet almost never goes directly from your computer to the destination.
Instead, it hops through several stops along the way. Each stop is called a
**router**.

```
Your          Router      Router      Router      Google's
Computer  -->   A    -->    B    -->    C    -->  Computer
              (Home)     (ISP)      (Google)

  "I need to        "I know the     "I know       "Packet
   get to Google.    way to Google.   the way.      arrived!"
   Router A is       Router B is     Google is
   my first stop."   next."          next."
```

Each router looks at the "To" address on the packet and decides where to send
it next. It is like asking for directions at every intersection.

The packet might travel through 10 or 15 routers before it arrives. Each one
brings it a little closer to the destination.

---

## What Unheaded Adds

Now here is where it gets interesting. Remember the tracking sticker from the
last chapter? This is where it goes.

When a packet enters an Unheaded-managed network, the **Shield** program
adds an extra section to the outside of the envelope. This section is called
the **Monad**.

Here is what the packet looks like before and after:

```
BEFORE (normal packet):

+------------------------------------------+
|  From: fd00::1    To: fd00::2            |  <-- IPv6 Header (40 bytes)
+------------------------------------------+
|  TCP Port 443 --> Port 8080              |  <-- Transport Header
+------------------------------------------+
|  <html>Google...</html>                  |  <-- Payload
+------------------------------------------+


AFTER (Unheaded packet):

+------------------------------------------+
|  From: fd00::1    To: fd00::2            |  <-- IPv6 Header (40 bytes)
+------------------------------------------+
| *** MONAD TRACKING STICKER (24 bytes) ***|  <-- Added by Shield
|  Version: 1                              |
|  Source Service: web-server              |
|  Destination: dashboard                  |
|  Hop Count: 0                            |
|  Quality Class: gold                     |
|  Checksum: 0xA7B3                        |
+------------------------------------------+
|  TCP Port 443 --> Port 8080              |  <-- Transport Header
+------------------------------------------+
|  <html>Google...</html>                  |  <-- Payload (UNTOUCHED)
+------------------------------------------+
```

Notice something important: the payload -- the actual message inside the
packet -- is completely untouched. The Monad sits between the address header
and the message, like putting a sticky note on the outside of the envelope.
The programs that read and write the Monad have no way to access the payload.

---

## The Journey of a Tracked Packet

Let us follow a single packet through an Unheaded network:

### Step 1: Birth

The packet arrives at the network boundary. The Shield program:
- Checks if the sender is allowed
- Puts a Monad tracking sticker on the packet
- Gives the packet a unique tracking number (Flow Label)
- Records: "Packet born at 3:15:00.001 PM"

### Step 2: First Hop

The packet reaches the first router inside the network. The eBPF program at
this router:
- Reads the Monad
- Adds 1 to the hop count ("this packet has been to 1 stop now")
- Checks the quality-of-service class
- Records: "Packet passed through Router A at 3:15:00.002 PM"
- Passes the packet to the next router

### Step 3: More Hops

The same thing happens at every stop. Each one reads the Monad, updates it,
records the visit, and passes the packet along. The hop count goes up by 1
at each stop.

### Step 4: Delivery

The packet arrives at the destination service. The application reads the
payload (the actual message) and does its job.

### Step 5: Death

When the packet leaves the Unheaded network, the Shield at the exit:
- Reads the final Monad state
- Records: "Packet died at 3:15:00.005 PM, visited 4 hops, no anomalies"
- Removes the Monad from the packet
- Sends the packet out as a normal envelope

The packet that leaves the network looks exactly like the packet that entered.
No trace of the Monad remains. The outside world never sees any of Unheaded's
tracking.

---

## The Logbook

All those "recordings" from each step go to a central logbook called
**Anamnesis** (a fancy word for "remembering").

Anamnesis collects events from every hop in the network. It is like a giant
notebook where every router writes down:

- What time the packet came through
- What the tracking sticker said
- Whether anything looked wrong

These events flow through **Wotan** (the message bus) to the **Dashboard**,
where the network operators can see everything in real time.

```
Shield (Birth)  ---+
                   |
Router A (Hop)  ---+--> Anamnesis --> Wotan --> Dashboard --> Operator's Screen
                   |
Router B (Hop)  ---+
                   |
Shield (Death)  ---+
```

---

## Why 20 Bytes?

The Monad tracking sticker is exactly 20 bytes. That is incredibly small.
For comparison:

- A single emoji is 4 bytes
- The word "hello" is 5 bytes
- The Monad is 20 bytes -- five "hello"s

In those 20 bytes, Unheaded packs:

| What | Size | Purpose |
|------|------|---------|
| Version | 1 byte | Which version of the protocol |
| Source service ID | 1 byte | Who sent this |
| Destination service ID | 1 byte | Where it is going |
| Hop count | 1 byte | How many stops so far |
| Quality class | 1 byte | How important is this packet |
| Flow action | 1 byte | What should each hop do with it |
| Circuit state | 1 byte | Is the connection healthy |
| Flags | 1 byte | Special markers (8 on/off switches) |
| Latency hint | 2 bytes | Expected delivery time |
| Deploy ring | 1 byte | Which environment (production, test) |
| Mesh flags | 1 byte | Service mesh routing info |
| Source prefix | 1 byte | Part of the sender's address |
| Destination prefix | 1 byte | Part of the receiver's address |
| Scratch registers | 4 bytes | General-purpose workspace |
| Checksum | 2 bytes | Error detection (is the sticker damaged?) |

**Total: 20 bytes.**

Every field has a purpose. Nothing is wasted. The checksum at the end makes
sure the sticker itself was not damaged in transit. If any bit gets flipped,
the checksum will not match, and the system will flag it as an anomaly.

---

## The Short Version

- Packets are digital envelopes with an address on the outside and a message
  on the inside.
- The network reads the address to deliver the packet, hopping through
  several routers along the way.
- Unheaded adds a 20-byte tracking sticker (Monad) to the outside of each
  packet when it enters the network.
- At every hop, a program reads and updates the sticker.
- When the packet leaves the network, the sticker is removed.
- All the tracking data flows to a dashboard for real-time visibility.
- The payload (the actual message) is never touched.

---

*Next: [Networking Basics](../middle-school/networking-basics.md) -- Ready to
learn the real terminology?*
