// SPDX-License-Identifier: GPL-2.0-or-later
// w_file_stdc.c — WAD I/O for bare-metal MBC (local override)
//
// Based on doomgeneric/w_file_stdc.c (Copyright Id Software / Simon Howard).
// Key change: uses STATIC allocation for stdc_wad_file_t instead of Z_Malloc.
// On MBC, Z_Malloc'd memory gets corrupted by zone management operations,
// destroying the file_class pointer and causing W_ReadLump to fail.

#include <stdio.h>

#include "m_misc.h"
#include "w_file.h"
#include "z_zone.h"

typedef struct
{
    wad_file_t wad;
    FILE *fstream;
} stdc_wad_file_t;

extern wad_file_class_t stdc_wad_file;

// Static storage for WAD file handles — immune to Z_Malloc corruption.
// Doom opens at most 2-3 WAD files (IWAD + optional PWADs).
#define MAX_WAD_FILES 4
static stdc_wad_file_t wad_file_slots[MAX_WAD_FILES];
static int wad_file_slot_used[MAX_WAD_FILES];

static wad_file_t *W_StdC_OpenFile(char *path)
{
    stdc_wad_file_t *result;
    FILE *fstream;
    int i;

    fstream = fopen(path, "rb");

    if (fstream == NULL)
    {
        return NULL;
    }

    // Find a free static slot (NOT Z_Malloc — immune to heap corruption)
    result = NULL;
    for (i = 0; i < MAX_WAD_FILES; i++)
    {
        if (!wad_file_slot_used[i])
        {
            wad_file_slot_used[i] = 1;
            result = &wad_file_slots[i];
            break;
        }
    }

    if (result == NULL)
    {
        fclose(fstream);
        return NULL;
    }

    result->wad.file_class = &stdc_wad_file;
    result->wad.mapped = NULL;
    result->wad.length = M_FileLength(fstream);
    result->fstream = fstream;

    return &result->wad;
}

static void W_StdC_CloseFile(wad_file_t *wad)
{
    stdc_wad_file_t *stdc_wad;
    int i;

    stdc_wad = (stdc_wad_file_t *) wad;

    fclose(stdc_wad->fstream);

    // Mark the static slot as free
    for (i = 0; i < MAX_WAD_FILES; i++)
    {
        if (&wad_file_slots[i] == stdc_wad)
        {
            wad_file_slot_used[i] = 0;
            break;
        }
    }
}

// Read data from the specified position in the file into the
// provided buffer.  Returns the number of bytes read.

size_t W_StdC_Read(wad_file_t *wad, unsigned int offset,
                   void *buffer, size_t buffer_len)
{
    stdc_wad_file_t *stdc_wad;
    size_t result;

    stdc_wad = (stdc_wad_file_t *) wad;

    // Jump to the specified position in the file.

    fseek(stdc_wad->fstream, offset, SEEK_SET);

    // Read into the buffer.

    result = fread(buffer, 1, buffer_len, stdc_wad->fstream);

    return result;
}


wad_file_class_t stdc_wad_file =
{
    W_StdC_OpenFile,
    W_StdC_CloseFile,
    W_StdC_Read,
};
