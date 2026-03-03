#ifndef DOOMGENERIC_H
#define DOOMGENERIC_H

#include <stdint.h>

// Screen dimensions (full DOOM resolution)
#define DOOMGENERIC_RESX 320
#define DOOMGENERIC_RESY 200

// Framebuffer: 320x200 8-bit palette-indexed pixels (CMAP256 mode)
extern uint32_t* DG_ScreenBuffer;

// Platform functions (implemented in doomgeneric_monad.c)
void DG_Init(void);
void DG_DrawFrame(void);
int  DG_GetKey(int* pressed, unsigned char* key);
void DG_SetWindowTitle(const char* title);
uint32_t DG_GetTicksMs(void);
void DG_SleepMs(uint32_t ms);

#endif
