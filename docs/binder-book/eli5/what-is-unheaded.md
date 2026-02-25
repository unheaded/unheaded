# What Is Unheaded?

*Explain Like I'm 5*

---

## The Post Office

Imagine you live in a big neighborhood. Everyone in the neighborhood sends
letters to each other all the time. There is a post office in the middle that
handles all the mail.

The post office has some important jobs:

1. **Sorting the mail.** Every letter has an address on it. The post office
   reads the address and puts the letter in the right bag for delivery.

2. **Keeping track.** The post office writes down every letter that comes
   through: who sent it, who it is going to, when it arrived, and when it left.
   This way, if a letter goes missing, they can figure out where it went wrong.

3. **Keeping things safe.** The post office has rules. You cannot send a bomb
   through the mail. You cannot open someone else's letters. You cannot pretend
   to be someone you are not.

**Unheaded is like that post office, but for computers.**

---

## What Computers Send

Computers do not send paper letters. They send little bundles of information
called **packets**. A packet is like a tiny digital envelope. It has:

- **An address** on the outside (so it knows where to go)
- **A return address** (so the other computer can write back)
- **A message inside** (the actual information)

Every time you load a website, watch a video, or send a text message, your
computer is sending and receiving thousands of these little envelopes.

---

## The Big Problem

Here is the problem: most of the time, nobody is watching the mail.

Imagine the post office just threw letters into bags and hoped for the best.
No tracking. No records. No way to know if a letter was lost, delayed, or
tampered with.

That is how most computer networks work today. The data gets sent, and
everyone just hopes it arrives. If something goes wrong, the people running
the network have to guess what happened. It is like trying to find a lost
letter in a post office that keeps no records.

---

## What Unheaded Does

Unheaded watches the mail without reading it.

Think about that for a moment. The post office can track that a letter went
from Alice to Bob, that it arrived at 3:15 PM, and that it took 2 minutes to
get there -- all without ever opening the envelope.

Unheaded does the same thing for computer networks:

1. **It stamps every envelope.** When a packet enters the network, Unheaded
   puts a tiny tracking sticker on it. This sticker has a unique number so
   that particular packet can be followed on its journey.

2. **It watches every step.** At each stop along the way, Unheaded reads the
   tracking sticker and writes down what happened: "Packet #12345 passed
   through here at 3:15 PM."

3. **It keeps a logbook.** All of those notes get collected into a big
   logbook. The people running the network can look at the logbook and see
   exactly what happened to every single packet.

4. **It locks the doors.** Unheaded has rules about who can send what and
   where. If a packet breaks the rules, it gets stopped at the door.

---

## The Really Important Part

Here is the part that makes Unheaded different from a nosy neighbor:

**Unheaded never reads the letters.**

It can see the envelopes. It can see the addresses. It can see how fast the
mail is moving. But it never, ever opens the envelopes to read what is inside.

This is not just a promise. It is built into the way the system works. The
tracking sticker goes on the outside of the envelope. The programs that read
the sticker physically cannot access the contents inside. It is like having a
mail scanner that can see the outside of the envelope but has no arms to open
it.

This is called **architectural isolation**. The system is designed so that
reading the contents is not just forbidden -- it is impossible.

---

## The Address Book

Every post office needs an address book. Unheaded has one too. It is called
**Wotan** (rhymes with "boat on").

Wotan is like the big bulletin board at the post office. When something
happens -- a new letter arrives, a mailbox is full, a delivery truck breaks
down -- a note gets pinned to the board. Everyone who needs to know can read
the board.

This is how all the different parts of Unheaded talk to each other. Instead
of shouting across the room, they write notes on the board. This keeps things
orderly and makes sure nobody misses an important message.

---

## The Stamps

The tracking stickers that Unheaded puts on packets are called **Monads**
(rhymes with "go dads").

A Monad is a tiny tag -- only 20 bytes, which is like 20 letters of the
alphabet. But packed into those 20 bytes is everything Unheaded needs to know:

- Who sent this packet?
- Where is it going?
- How many stops has it made?
- Is there anything unusual about it?
- A little checksum to make sure the sticker itself has not been damaged.

The Monad rides along with the packet from the moment it enters the network
until the moment it leaves. At each stop, the programs at that stop can read
the Monad and update it.

---

## The Big Board

All those tracking notes need to go somewhere useful. That is the
**Dashboard**.

The Dashboard is a screen that shows the network operators what is happening
right now. You can think of it as the big status board at the post office:

- How many letters are in the system right now?
- Are any delivery routes running slow?
- Has anything unusual happened?
- What is the busiest mailbox?

The Dashboard updates in real time. When a packet enters the network, it
shows up on the board within a fraction of a second.

---

## The Rules

Unheaded has a **Shield** at the boundary of the network. Think of it as the
security guard at the front door of the post office.

When a letter comes in from outside:

1. The Shield checks if the sender is allowed to send mail here.
2. It removes any sketchy-looking packaging (extra headers that might confuse
   the system).
3. It puts the Monad tracking sticker on the envelope.
4. It lets the letter through.

When a letter goes out:

1. The Shield reads the final tracking sticker to record what happened during
   the letter's journey.
2. It removes the tracking sticker (outsiders do not need to see internal
   tracking information).
3. It releases the letter into the outside world as a normal envelope.

Nothing with a tracking sticker ever leaves the building. Nothing from
outside enters without getting one.

---

## Why Does This Matter?

Because when things go wrong with computer networks -- and they always do --
the people running the network need to know what happened.

Without Unheaded: "Something broke. We think it was the database but maybe
it was the network. We will spend the next four hours guessing."

With Unheaded: "Packet #12345 left Service A at 3:15:00.001 PM, arrived at
Service B at 3:15:00.004 PM, and Service B rejected it because of a timeout.
The timeout was caused by Service C not responding. Service C has been
restarted."

That is the difference between guessing and knowing.

---

## The Short Version

- Computers send tiny digital envelopes called packets.
- Most networks do not track those packets well.
- Unheaded tracks every packet from the moment it enters the network to the
  moment it leaves.
- It does this by putting a tiny tracking sticker (Monad) on each packet.
- It never reads the contents of the packets -- only the envelopes.
- All the tracking information goes to a dashboard where operators can see
  exactly what is happening.
- A guard (Shield) at the border stamps packets coming in and removes stamps
  from packets going out.

That is Unheaded: the world's most thorough, most private post office.

---

*Next: [How Packets Work](how-packets-work.md) -- What is actually inside
those digital envelopes?*
