// SPDX-License-Identifier: GPL-3.0-or-later
// i_net_mbc.c — MBC network stubs for id DOOM (single-player only)
//
// Replaces linuxdoom-1.10/i_net.c (BSD sockets) with single-player stubs.

#include <stdlib.h>
#include <string.h>

#include "doomdef.h"
#include "doomstat.h"
#include "d_net.h"
#include "m_argv.h"
#include "i_system.h"

// These are defined in d_net.c / g_game.c
extern doomcom_t *doomcom;

void I_InitNetwork(void)
{
    doomcom = malloc(sizeof(*doomcom));
    memset(doomcom, 0, sizeof(*doomcom));

    // Single-player configuration
    doomcom->id = DOOMCOM_ID;
    doomcom->ticdup = 1;
    doomcom->extratics = 0;
    doomcom->numnodes = 1;
    doomcom->numplayers = 1;
    doomcom->consoleplayer = 0;
    doomcom->deathmatch = 0;

    netgame = false;
}

void I_NetCmd(void)
{
    I_Error("I_NetCmd called in single player");
}
