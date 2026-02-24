/*
 * libc_monad.c - Minimal bare-metal libc for Doom on MBC VM
 *
 * Fixes applied:
 *   S31-F1: WAD file I/O (fread/fseek/ftell/open/read/lseek)
 *   S31-F2: Inline byte WAD matching (no .rodata dependency)
 *   S31-F3: Debug buffer expanded to ~128KB
 *   S31-F4: rewind() / feof() WAD-aware
 */
#include <stddef.h>
#include <stdint.h>
#include <stdarg.h>

void *memcpy(void *dst, const void *src, size_t n);
void *memset(void *s, int c, size_t n);
size_t strlen(const char *s);
int isdigit(int c);
int isspace(int c);
int atoi(const char *s);

typedef struct { int dummy; } FILE;
extern FILE *stdin;
extern FILE *stdout;
extern FILE *stderr;
#define EOF (-1)

/* ========== WAD MEMORY-MAPPED I/O ========== */
#define WAD_BASE ((unsigned char *)0x00110000)
#define WAD_SIZE 4196020        /* doom1.wad — update if switching WADs */
#define WAD_FD   42             /* fake fd for open()/read()/lseek() */
static long _wad_fpos  = 0;    /* stdio file position (fopen/fread) */
static long _wad_fdpos = 0;    /* Unix fd position (open/read/lseek) */

/* ========== ALLOCATOR ========== */
#define HEAP_BASE  ((char *)0x00520000)
#define HEAP_SIZE  (16 * 1024 * 1024)
#define HEAP_END   (HEAP_BASE + HEAP_SIZE)
static char *heap_ptr = HEAP_BASE;

void *malloc(size_t size) {
    char *p;
    if (size == 0) return NULL;
    size = (size + 7) & ~(size_t)7;
    if (heap_ptr + size > HEAP_END) return NULL;
    p = heap_ptr; heap_ptr += size; return (void *)p;
}
void free(void *ptr) { (void)ptr; }
void *calloc(size_t nmemb, size_t size) {
    size_t total = nmemb * size;
    void *p = malloc(total);
    if (p) memset(p, 0, total);
    return p;
}
void *realloc(void *ptr, size_t size) {
    void *newp;
    if (!ptr) return malloc(size);
    if (size == 0) { free(ptr); return NULL; }
    newp = malloc(size);
    if (newp) memcpy(newp, ptr, size);
    return newp;
}

/* ========== STRING ========== */
void *memcpy(void *dst, const void *src, size_t n) {
    unsigned char *d = (unsigned char *)dst;
    const unsigned char *s = (const unsigned char *)src;
    while (n--) *d++ = *s++; return dst;
}
void *memmove(void *dst, const void *src, size_t n) {
    unsigned char *d = (unsigned char *)dst;
    const unsigned char *s = (const unsigned char *)src;
    if (d < s) { while (n--) *d++ = *s++; }
    else if (d > s) { d += n; s += n; while (n--) *--d = *--s; }
    return dst;
}
void *memset(void *s, int c, size_t n) {
    unsigned char *p = (unsigned char *)s;
    while (n--) *p++ = (unsigned char)c; return s;
}
int memcmp(const void *s1, const void *s2, size_t n) {
    const unsigned char *a = s1, *b = s2;
    while (n--) { if (*a != *b) return *a - *b; a++; b++; }
    return 0;
}
void *memchr(const void *s, int c, size_t n) {
    const unsigned char *p = s;
    while (n--) { if (*p == (unsigned char)c) return (void *)p; p++; }
    return NULL;
}
size_t strlen(const char *s) { const char *p = s; while (*p) p++; return (size_t)(p - s); }
int strcmp(const char *s1, const char *s2) { while (*s1 && *s1 == *s2) { s1++; s2++; } return (unsigned char)*s1 - (unsigned char)*s2; }
int strncmp(const char *s1, const char *s2, size_t n) { while (n && *s1 && *s1 == *s2) { s1++; s2++; n--; } return n ? (unsigned char)*s1 - (unsigned char)*s2 : 0; }
char *strcpy(char *dst, const char *src) { char *d = dst; while ((*d++ = *src++)); return dst; }
char *strncpy(char *dst, const char *src, size_t n) { char *d = dst; while (n && (*d++ = *src++)) n--; while (n--) *d++ = '\0'; return dst; }
char *strcat(char *dst, const char *src) { char *d = dst; while (*d) d++; while ((*d++ = *src++)); return dst; }
char *strncat(char *dst, const char *src, size_t n) { char *d = dst; while (*d) d++; while (n-- && (*d = *src++)) d++; *d = '\0'; return dst; }
char *strchr(const char *s, int c) { while (*s) { if (*s == (char)c) return (char *)s; s++; } return (c == '\0') ? (char *)s : NULL; }
char *strrchr(const char *s, int c) { const char *last = NULL; while (*s) { if (*s == (char)c) last = s; s++; } return c == '\0' ? (char *)s : (char *)last; }
char *strstr(const char *h, const char *n) { size_t nl; if (!*n) return (char *)h; nl = strlen(n); while (*h) { if (strncmp(h, n, nl) == 0) return (char *)h; h++; } return NULL; }
char *strdup(const char *s) { size_t l = strlen(s)+1; char *d = malloc(l); if (d) memcpy(d, s, l); return d; }
static int _tl(int c) { return (c>='A'&&c<='Z') ? c+32 : c; }
static int _tu(int c) { return (c>='a'&&c<='z') ? c-32 : c; }
int strcasecmp(const char *s1, const char *s2) { while (*s1 && _tl(*s1)==_tl(*s2)) { s1++; s2++; } return _tl((unsigned char)*s1)-_tl((unsigned char)*s2); }
int strncasecmp(const char *s1, const char *s2, size_t n) { while (n && *s1 && _tl(*s1)==_tl(*s2)) { s1++; s2++; n--; } return n ? _tl((unsigned char)*s1)-_tl((unsigned char)*s2) : 0; }
char *strerror(int e) { (void)e; return "error"; }
size_t strspn(const char *s, const char *a) { const char *p=s; while(*p){const char *q=a;int f=0;while(*q){if(*p==*q){f=1;break;}q++;}if(!f)break;p++;} return (size_t)(p-s); }
size_t strcspn(const char *s, const char *r) { const char *p=s; while(*p){const char *q=r;while(*q){if(*p==*q)return(size_t)(p-s);q++;}p++;} return (size_t)(p-s); }
static char *_strtok_s;
char *strtok(char *str, const char *d) { char *st; if(str)_strtok_s=str; if(!_strtok_s)return NULL; _strtok_s+=strspn(_strtok_s,d); if(!*_strtok_s)return NULL; st=_strtok_s; _strtok_s+=strcspn(_strtok_s,d); if(*_strtok_s)*_strtok_s++='\0'; return st; }

/* ========== CTYPE ========== */
int isalpha(int c) { return (c>='A'&&c<='Z')||(c>='a'&&c<='z'); }
int isdigit(int c) { return c>='0'&&c<='9'; }
int isalnum(int c) { return isalpha(c)||isdigit(c); }
int isspace(int c) { return c==' '||c=='\t'||c=='\n'||c=='\r'||c=='\f'||c=='\v'; }
int isupper(int c) { return c>='A'&&c<='Z'; }
int islower(int c) { return c>='a'&&c<='z'; }
int isprint(int c) { return c>=0x20&&c<=0x7E; }
int ispunct(int c) { return isprint(c)&&!isalnum(c)&&!isspace(c); }
int isxdigit(int c) { return isdigit(c)||(c>='a'&&c<='f')||(c>='A'&&c<='F'); }
int iscntrl(int c) { return (c>=0&&c<0x20)||c==0x7F; }
int isgraph(int c) { return c>0x20&&c<=0x7E; }
int toupper(int c) { return _tu(c); }
int tolower(int c) { return _tl(c); }

/* ========== NUMBERS ========== */
int atoi(const char *s) { int r=0,sg=1; while(isspace(*s))s++; if(*s=='-'){sg=-1;s++;}else if(*s=='+')s++; while(isdigit(*s)){r=r*10+(*s-'0');s++;} return sg*r; }
long atol(const char *s) { return (long)atoi(s); }
long strtol(const char *s, char **ep, int b) {
    long r=0; int sg=1;
    while(isspace(*s))s++;
    if(*s=='-'){sg=-1;s++;}else if(*s=='+')s++;
    if(b==0){if(*s=='0'){s++;if(*s=='x'||*s=='X'){b=16;s++;}else b=8;}else b=10;}
    else if(b==16&&*s=='0'&&(s[1]=='x'||s[1]=='X'))s+=2;
    while(*s){int d;if(isdigit(*s))d=*s-'0';else if(*s>='a'&&*s<='f')d=*s-'a'+10;else if(*s>='A'&&*s<='F')d=*s-'A'+10;else break;if(d>=b)break;r=r*b+d;s++;}
    if(ep)*ep=(char*)s; return sg*r;
}
unsigned long strtoul(const char *s, char **ep, int b) { return (unsigned long)strtol(s,ep,b); }
double atof(const char *s) { double r=0,fr=0,dv=1; int sg=1,inf=0; while(isspace(*s))s++; if(*s=='-'){sg=-1;s++;}else if(*s=='+')s++; while(*s){if(isdigit(*s)){if(inf){dv*=10;fr+=(*s-'0')/dv;}else r=r*10+(*s-'0');}else if(*s=='.'&&!inf)inf=1;else break;s++;} return sg*(r+fr); }
int abs(int x) { return x<0?-x:x; }
long labs(long x) { return x<0?-x:x; }

/* ========== QSORT ========== */
static void _swp(char *a, char *b, size_t sz) { char t; while(sz--){t=*a;*a++=*b;*b++=t;} }
void qsort(void *base, size_t n, size_t sz, int(*cmp)(const void*,const void*)) {
    size_t i,j; char *a=(char*)base;
    for(i=1;i<n;i++){j=i;while(j>0&&cmp(a+j*sz,a+(j-1)*sz)<0){_swp(a+j*sz,a+(j-1)*sz,sz);j--;}}
}
void *bsearch(const void *key, const void *base, size_t n, size_t sz, int(*cmp)(const void*,const void*)) {
    size_t lo=0,hi=n; const char *a=base;
    while(lo<hi){size_t m=lo+(hi-lo)/2;int c=cmp(key,a+m*sz);if(c==0)return(void*)(a+m*sz);if(c<0)hi=m;else lo=m+1;}
    return NULL;
}

/* ========== PRINTF ========== */
static int _pn(char *buf, int bm, int *pos, unsigned val, int base, int is, int w, int pz, int up) {
    char tmp[12]; int i=0,neg=0; int iv=(int)val;
    if(is&&iv<0){neg=1;val=(unsigned)(-iv);}
    if(val==0)tmp[i++]='0'; else{while(val){int d=val%base;tmp[i++]=d<10?'0'+d:(up?'A':'a')+d-10;val/=base;}}
    if(neg)tmp[i++]='-';
    while(i<w)tmp[i++]=pz?'0':' ';
    while(i>0){i--;if(buf&&*pos<bm-1)buf[(*pos)++]=tmp[i];else(*pos)++;}
    return 0;
}
int vsnprintf(char *buf, size_t n, const char *fmt, va_list ap) {
    int pos=0,bm=(int)n;
    while(*fmt){
        if(*fmt!='%'){if(buf&&pos<bm-1)buf[pos]=*fmt;pos++;fmt++;continue;}
        fmt++;
        int pz=0,w=0,pr=-1;
        while(*fmt=='0'||*fmt=='-'||*fmt==' '||*fmt=='+'){if(*fmt=='0')pz=1;fmt++;}
        if(*fmt=='*'){w=va_arg(ap,int);fmt++;}else while(isdigit(*fmt)){w=w*10+(*fmt-'0');fmt++;}
        if(*fmt=='.'){fmt++;pr=0;if(*fmt=='*'){pr=va_arg(ap,int);fmt++;}else while(isdigit(*fmt)){pr=pr*10+(*fmt-'0');fmt++;}}
        if(*fmt=='l'){fmt++;if(*fmt=='l')fmt++;}else if(*fmt=='h'){fmt++;if(*fmt=='h')fmt++;}else if(*fmt=='z')fmt++;
        switch(*fmt){
        case'd':case'i':{int v=va_arg(ap,int);_pn(buf,bm,&pos,(unsigned)v,10,1,w,pz,0);break;}
        case'u':{unsigned v=va_arg(ap,unsigned);_pn(buf,bm,&pos,v,10,0,w,pz,0);break;}
        case'x':{unsigned v=va_arg(ap,unsigned);_pn(buf,bm,&pos,v,16,0,w,pz,0);break;}
        case'X':{unsigned v=va_arg(ap,unsigned);_pn(buf,bm,&pos,v,16,0,w,pz,1);break;}
        case'o':{unsigned v=va_arg(ap,unsigned);_pn(buf,bm,&pos,v,8,0,w,pz,0);break;}
        case'p':{unsigned v=(unsigned)(uintptr_t)va_arg(ap,void*);if(buf&&pos<bm-1)buf[pos]='0';pos++;if(buf&&pos<bm-1)buf[pos]='x';pos++;_pn(buf,bm,&pos,v,16,0,8,1,0);break;}
        case's':{const char*s=va_arg(ap,const char*);if(!s)s="(null)";int sl=(int)strlen(s);if(pr>=0&&pr<sl)sl=pr;int pd=w>sl?w-sl:0;int si;while(pd-->0){if(buf&&pos<bm-1)buf[pos]=' ';pos++;}for(si=0;si<sl;si++){if(buf&&pos<bm-1)buf[pos]=s[si];pos++;}break;}
        case'c':{int c=va_arg(ap,int);if(buf&&pos<bm-1)buf[pos]=(char)c;pos++;break;}
        case'f':{double v=va_arg(ap,double);int iv2;if(v<0){if(buf&&pos<bm-1)buf[pos]='-';pos++;v=-v;}iv2=(int)v;_pn(buf,bm,&pos,(unsigned)iv2,10,0,0,0,0);if(buf&&pos<bm-1)buf[pos]='.';pos++;int fr=(int)((v-iv2)*100);if(fr<0)fr=-fr;if(fr<10){if(buf&&pos<bm-1)buf[pos]='0';pos++;}_pn(buf,bm,&pos,(unsigned)fr,10,0,0,0,0);break;}
        case'%':if(buf&&pos<bm-1)buf[pos]='%';pos++;break;
        case'\0':goto done;
        default:if(buf&&pos<bm-1)buf[pos]=*fmt;pos++;break;
        }
        fmt++;
    }
done:
    if(buf){if(pos<bm)buf[pos]='\0';else if(bm>0)buf[bm-1]='\0';}
    return pos;
}
int snprintf(char*b,size_t n,const char*f,...){va_list a;int r;va_start(a,f);r=vsnprintf(b,n,f,a);va_end(a);return r;}
int vsprintf(char*b,const char*f,va_list a){return vsnprintf(b,0x7FFFFFFF,f,a);}
int sprintf(char*b,const char*f,...){va_list a;int r;va_start(a,f);r=vsprintf(b,f,a);va_end(a);return r;}

/* ========== DEBUG OUTPUT (~128KB buffer) ========== */
#define DBG_BUF  ((volatile char *)0x0F0000)
#define DBG_LEN  ((volatile unsigned int *)0x10FFFC)
#define DBG_CAP  (0x10FFFC - 0x0F0000)
static void dbg_puts(const char *s) {
    unsigned int pos = *DBG_LEN;
    while (*s && pos < DBG_CAP) { DBG_BUF[pos++] = *s++; }
    DBG_BUF[pos] = '\0';
    *DBG_LEN = pos;
}
int vfprintf(FILE*f,const char*fm,va_list a){(void)f;char t[512];int r=vsnprintf(t,sizeof(t),fm,a);dbg_puts(t);return r;}
int fprintf(FILE*f,const char*fm,...){va_list a;int r;va_start(a,fm);r=vfprintf(f,fm,a);va_end(a);return r;}
int vprintf(const char*f,va_list a){return vfprintf(stdout,f,a);}
int printf(const char*f,...){va_list a;int r;va_start(a,f);r=vprintf(f,a);va_end(a);return r;}
int sscanf(const char*s,const char*f,...){va_list a;int c=0;va_start(a,f);while(*f&&*s){if(*f=='%'){f++;if(*f=='d'||*f=='i'){int*ip=va_arg(a,int*);*ip=atoi(s);while(isspace(*s))s++;if(*s=='-'||*s=='+')s++;while(isdigit(*s))s++;c++;f++;}else if(*f=='s'){char*sp=va_arg(a,char*);while(*s&&!isspace(*s))*sp++=*s++;*sp='\0';c++;f++;}else f++;}else{if(*f==*s){f++;s++;}else break;}}va_end(a);return c;}
int puts(const char*s){dbg_puts(s);dbg_puts("\n");return 0;}
int putchar(int c){(void)c;return c;}
int fputc(int c,FILE*f){(void)c;(void)f;return c;}
int fputs(const char*s,FILE*f){(void)f;dbg_puts(s);return 0;}
int fflush(FILE*f){(void)f;return 0;}

/* ========== INLINE WAD FILENAME MATCHING ========== */
/*
 * Matches "doom1.wad" (case-insensitive) at the END of the path.
 * Uses inline byte comparisons — ZERO dependency on .rodata strings.
 * This avoids the .rodata corruption bug that plagued strcasecmp-based matching.
 */
static int _tail_match_doom1_wad(const char *p) {
    /* Match exactly "doom1.wad" (9 chars) at end of string */
    if (!p) return 0;
    size_t len = strlen(p);
    if (len < 9) return 0;
    const char *t = p + len - 9;
    /* Verify no extra prefix chars (or preceded by path separator) */
    if (len > 9 && t[-1] != '/' && t[-1] != '\\') return 0;
    return (_tl(t[0])=='d' && _tl(t[1])=='o' && _tl(t[2])=='o' && _tl(t[3])=='m'
         && t[4]=='1' && t[5]=='.'
         && _tl(t[6])=='w' && _tl(t[7])=='a' && _tl(t[8])=='d');
}

/* ========== FILE I/O (WAD-aware) ========== */
static FILE _si,_so,_se;
FILE*stdin=&_si;FILE*stdout=&_so;FILE*stderr=&_se;
static FILE _wad_f;

FILE*fopen(const char*p,const char*m){
    (void)m;
    if(_tail_match_doom1_wad(p)){_wad_fpos=0;return &_wad_f;}
    return NULL;
}
FILE*freopen(const char*p,const char*m,FILE*f){(void)p;(void)m;(void)f;return NULL;}
int fclose(FILE*f){(void)f;return 0;}

size_t fread(void*p,size_t s,size_t n,FILE*f){
    if(f==&_wad_f && s>0){
        size_t total=s*n;
        if(_wad_fpos>=(long)WAD_SIZE)return 0;
        if((size_t)_wad_fpos+total>WAD_SIZE)total=WAD_SIZE-(size_t)_wad_fpos;
        memcpy(p,WAD_BASE+_wad_fpos,total);
        _wad_fpos+=(long)total;
        return total/s;
    }
    return 0;
}
size_t fwrite(const void*p,size_t s,size_t n,FILE*f){(void)p;(void)f;return s*n;}
int fseek(FILE*f,long o,int w){
    if(f==&_wad_f){
        if(w==0)_wad_fpos=o;
        else if(w==1)_wad_fpos+=o;
        else if(w==2)_wad_fpos=(long)WAD_SIZE+o;
        return 0;
    }
    return -1;
}
long ftell(FILE*f){if(f==&_wad_f)return _wad_fpos;return 0;}
void rewind(FILE*f){if(f==&_wad_f)_wad_fpos=0;}
int feof(FILE*f){if(f==&_wad_f)return _wad_fpos>=(long)WAD_SIZE;return 1;}
int ferror(FILE*f){(void)f;return 0;}
int fgetc(FILE*f){
    if(f==&_wad_f && _wad_fpos<(long)WAD_SIZE){
        return (unsigned char)WAD_BASE[_wad_fpos++];
    }
    return EOF;
}
char*fgets(char*s,int n,FILE*f){
    if(f==&_wad_f && n>0 && _wad_fpos<(long)WAD_SIZE){
        int i=0;
        while(i<n-1 && _wad_fpos<(long)WAD_SIZE){
            char c=(char)WAD_BASE[_wad_fpos++];
            s[i++]=c;
            if(c=='\n')break;
        }
        s[i]='\0';
        return s;
    }
    return NULL;
}
int ungetc(int c,FILE*f){
    if(f==&_wad_f && _wad_fpos>0){_wad_fpos--;return c;}
    return EOF;
}
int fileno(FILE*f){if(f==&_wad_f)return WAD_FD;return-1;}
int rename(const char*o,const char*n){(void)o;(void)n;return-1;}
int remove(const char*p){(void)p;return-1;}
void perror(const char*s){if(s)dbg_puts(s);}
FILE*tmpfile(void){return NULL;}
char*tmpnam(char*s){(void)s;return NULL;}
void clearerr(FILE*f){(void)f;}
void setbuf(FILE*f,char*b){(void)f;(void)b;}
int setvbuf(FILE*f,char*b,int m,size_t s){(void)f;(void)b;(void)m;(void)s;return 0;}

/* ========== UNIX I/O (WAD-aware) ========== */
int open(const char*p,int fl,...){
    (void)fl;
    if(_tail_match_doom1_wad(p)){_wad_fdpos=0;return WAD_FD;}
    return-1;
}
int close(int fd){(void)fd;return 0;}
typedef long ssize_t_monad;
typedef long off_t_monad;
ssize_t_monad read(int fd,void*buf,size_t count){
    if(fd==WAD_FD){
        if(_wad_fdpos>=(long)WAD_SIZE)return 0;
        if((size_t)_wad_fdpos+count>WAD_SIZE)count=WAD_SIZE-(size_t)_wad_fdpos;
        memcpy(buf,WAD_BASE+_wad_fdpos,count);
        _wad_fdpos+=(long)count;
        return(ssize_t_monad)count;
    }
    return-1;
}
off_t_monad lseek(int fd,off_t_monad off,int whence){
    if(fd==WAD_FD){
        if(whence==0)_wad_fdpos=off;
        else if(whence==1)_wad_fdpos+=off;
        else if(whence==2)_wad_fdpos=(long)WAD_SIZE+off;
        return(off_t_monad)_wad_fdpos;
    }
    return-1;
}
int isatty(int fd){(void)fd;return 0;}
int access(const char*p,int m){
    (void)m;
    if(_tail_match_doom1_wad(p))return 0;
    return-1;
}
int unlink(const char*p){(void)p;return-1;}
unsigned int sleep(unsigned int s){(void)s;return 0;}
int usleep(unsigned int u){(void)u;return 0;}
char*getcwd(char*b,size_t s){if(b&&s>1){b[0]='.';b[1]='\0';}return b;}
int chdir(const char*p){(void)p;return-1;}
int stat(const char*p,void*b){(void)p;(void)b;return-1;}
int fstat(int fd,void*b){(void)fd;(void)b;return-1;}
int mkdir(const char*p,unsigned int m){(void)p;(void)m;return-1;}

/* ========== MATH ========== */
double floor(double x){int i=(int)x;return(double)(x<0&&x!=(double)i?i-1:i);}
double ceil(double x){int i=(int)x;return(double)(x>0&&x!=(double)i?i+1:i);}
double fabs(double x){return x<0?-x:x;}
double fmod(double x,double y){if(y==0.0)return 0.0;return x-(int)(x/y)*y;}
double sqrt(double x){double g;int i;if(x<=0.0)return 0.0;g=x;for(i=0;i<20;i++)g=(g+x/g)*0.5;return g;}
double sin(double x){double t=x,s=x;int i;for(i=1;i<10;i++){t*=-x*x/((2*i)*(2*i+1));s+=t;}return s;}
double cos(double x){double t=1.0,s=1.0;int i;for(i=1;i<10;i++){t*=-x*x/((2*i-1)*(2*i));s+=t;}return s;}
double tan(double x){double c=cos(x);if(c==0.0)return 0.0;return sin(x)/c;}
double atan(double x){if(x>1.0)return 1.5707963-atan(1.0/x);if(x<-1.0)return-1.5707963-atan(1.0/x);double t=x,s=x;int i;for(i=1;i<15;i++){t*=-x*x;s+=t/(2*i+1);}return s;}
double atan2(double y,double x){if(x>0)return atan(y/x);if(x<0&&y>=0)return atan(y/x)+3.14159265358979;if(x<0&&y<0)return atan(y/x)-3.14159265358979;if(y>0)return 1.5707963;if(y<0)return-1.5707963;return 0.0;}
double log(double x){(void)x;return 0.0;}
double exp(double x){(void)x;return 1.0;}
double pow(double x,double y){if(y==0.0)return 1.0;if(y==1.0)return x;if(y==2.0)return x*x;double r=1.0;int iy=(int)y,i;if((double)iy==y&&iy>0){for(i=0;i<iy;i++)r*=x;return r;}return 1.0;}
float floorf(float x){return(float)floor((double)x);}
float ceilf(float x){return(float)ceil((double)x);}
float sqrtf(float x){return(float)sqrt((double)x);}
float fabsf(float x){return x<0?-x:x;}

/* ========== MISC ========== */
int errno=0;
static unsigned _rs=1;
int rand(void){_rs=_rs*1103515245+12345;return(_rs>>16)&0x7FFF;}
void srand(unsigned s){_rs=s;}
char*getenv(const char*n){(void)n;return NULL;}
int system(const char*c){(void)c;return-1;}
long time(long*t){if(t)*t=0;return 0;}
long clock(void){return 0;}

typedef void(*sighandler_t)(int);
sighandler_t signal(int s,sighandler_t h){(void)s;(void)h;return(sighandler_t)0;}
int raise(int s){(void)s;return 0;}
int setjmp(int env[16]){(void)env;return 0;}
void longjmp(int env[16],int val){(void)env;(void)val;while(1);}
void __assert_fail(const char*e,const char*f,int l){(void)e;(void)f;(void)l;while(1);}

void exit(int status){(void)status;asm volatile("ebreak");__builtin_unreachable();}
void abort(void){exit(1);}
void __stack_chk_fail(void){while(1);}

/* ========== SOFT-FLOAT / 64-BIT STUBS ========== */
/* rv32im has no FPU — provide minimal stubs for libgcc functions */
long long __divdi3(long long a, long long b) {
    if (b == 0) return 0;
    int neg = 0;
    unsigned long long ua, ub, q;
    if (a < 0) { neg = !neg; ua = (unsigned long long)(-a); } else { ua = (unsigned long long)a; }
    if (b < 0) { neg = !neg; ub = (unsigned long long)(-b); } else { ub = (unsigned long long)b; }
    q = 0;
    unsigned long long bit;
    for (bit = 1ULL << 62; bit > 0; bit >>= 1) {
        if (ua >= ub * bit) { q += bit; ua -= ub * bit; }
    }
    return neg ? -(long long)q : (long long)q;
}
long long __moddi3(long long a, long long b) {
    return a - __divdi3(a, b) * b;
}
unsigned long long __udivdi3(unsigned long long a, unsigned long long b) {
    if (b == 0) return 0;
    unsigned long long q = 0, bit;
    for (bit = 1ULL << 62; bit > 0; bit >>= 1) {
        if (a >= b * bit) { q += bit; a -= b * bit; }
    }
    return q;
}
unsigned long long __umoddi3(unsigned long long a, unsigned long long b) {
    return a - __udivdi3(a, b) * b;
}

/* Soft-float stubs — all return 0 / do nothing.
 * Doom's core rendering is integer-only (fixed-point).
 * These are only called from non-critical paths (mouse speed box, etc.) */
typedef unsigned int su32;
typedef unsigned long long su64;
su32 __addsf3(su32 a, su32 b){(void)a;(void)b;return 0;}
su32 __subsf3(su32 a, su32 b){(void)a;(void)b;return 0;}
su32 __mulsf3(su32 a, su32 b){(void)a;(void)b;return 0;}
su32 __divsf3(su32 a, su32 b){(void)a;(void)b;return 0;}
int __lesf2(su32 a, su32 b){(void)a;(void)b;return 0;}
int __gesf2(su32 a, su32 b){(void)a;(void)b;return 0;}
int __ltsf2(su32 a, su32 b){(void)a;(void)b;return 0;}
int __gtsf2(su32 a, su32 b){(void)a;(void)b;return 0;}
int __eqsf2(su32 a, su32 b){(void)a;(void)b;return 0;}
int __nesf2(su32 a, su32 b){(void)a;(void)b;return 0;}
su32 __fixsfsi(su32 a){(void)a;return 0;}
su32 __floatsisf(int a){(void)a;return 0;}
su64 __extendsfdf2(su32 a){(void)a;return 0;}
su64 __adddf3(su64 a, su64 b){(void)a;(void)b;return 0;}
su64 __subdf3(su64 a, su64 b){(void)a;(void)b;return 0;}
su64 __muldf3(su64 a, su64 b){(void)a;(void)b;return 0;}
su64 __divdf3(su64 a, su64 b){(void)a;(void)b;return 0;}
int __ledf2(su64 a, su64 b){(void)a;(void)b;return 0;}
int __gedf2(su64 a, su64 b){(void)a;(void)b;return 0;}
int __ltdf2(su64 a, su64 b){(void)a;(void)b;return 0;}
int __gtdf2(su64 a, su64 b){(void)a;(void)b;return 0;}
int __eqdf2(su64 a, su64 b){(void)a;(void)b;return 0;}
int __nedf2(su64 a, su64 b){(void)a;(void)b;return 0;}
int __fixdfsi(su64 a){(void)a;return 0;}
su64 __floatsidf(int a){(void)a;return 0;}
su32 __truncdfsf2(su64 a){(void)a;return 0;}
int __unordsf2(su32 a, su32 b){(void)a;(void)b;return 0;}
int __unorddf2(su64 a, su64 b){(void)a;(void)b;return 0;}

