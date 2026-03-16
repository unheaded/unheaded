// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef _SIGNAL_H
#define _SIGNAL_H

#define SIGINT  2
#define SIGTERM 15

typedef void (*sig_t)(int);

sig_t signal(int sig, sig_t handler);

#endif
