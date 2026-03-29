// SPDX-License-Identifier: GPL-3.0-or-later
// i_sound_mbc.c — MBC sound stubs for id DOOM (all no-ops)
//
// No audio hardware on MBC. All functions are stubs.
// CRITICAL: I_SetChannels must NOT call pow() (would pull in libm).
// CRITICAL: I_GetSfxLumpNum must work (returns WAD lump number).

#include <stdio.h>
#include <string.h>

#include "z_zone.h"
#include "i_system.h"
#include "i_sound.h"
#include "m_argv.h"
#include "m_misc.h"
#include "w_wad.h"
#include "doomdef.h"

// ============================================================
// Sound effects
// ============================================================

void I_InitSound(void)
{
    // No sound hardware
}

void I_ShutdownSound(void)
{
}

void I_SetChannels(void)
{
    // Original calls pow() for volume lookup tables — we skip all of it.
    // No sound channels to configure on MBC.
}

void I_SetSfxVolume(int volume)
{
    (void)volume;
}

// Get the WAD lump number for a sound effect.
// This MUST work — s_sound.c calls it during startup to cache lump indices.
int I_GetSfxLumpNum(sfxinfo_t *sfxinfo)
{
    char namebuf[9];
    sprintf(namebuf, "ds%s", sfxinfo->name);
    return W_GetNumForName(namebuf);
}

int I_StartSound(int id, int vol, int sep, int pitch, int priority)
{
    (void)id; (void)vol; (void)sep; (void)pitch; (void)priority;
    return 0;
}

void I_StopSound(int handle)
{
    (void)handle;
}

int I_SoundIsPlaying(int handle)
{
    (void)handle;
    return 0;
}

void I_UpdateSound(void)
{
}

void I_SubmitSound(void)
{
}

void I_UpdateSoundParams(int handle, int vol, int sep, int pitch)
{
    (void)handle; (void)vol; (void)sep; (void)pitch;
}

// ============================================================
// Music
// ============================================================

void I_InitMusic(void)
{
}

void I_ShutdownMusic(void)
{
}

void I_SetMusicVolume(int volume)
{
    (void)volume;
}

void I_PlaySong(int handle, int looping)
{
    (void)handle; (void)looping;
}

void I_PauseSong(int handle)
{
    (void)handle;
}

void I_ResumeSong(int handle)
{
    (void)handle;
}

void I_StopSong(int handle)
{
    (void)handle;
}

void I_UnRegisterSong(int handle)
{
    (void)handle;
}

int I_RegisterSong(void *data)
{
    (void)data;
    return 0;
}

int I_QrySongPlaying(int handle)
{
    (void)handle;
    return 0;
}
