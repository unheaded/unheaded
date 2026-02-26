# SPDX-License-Identifier: MIT
# BIRD Dockerfile — builds from ~/tmp/bird-master/ source
# Multi-stage build for minimal final image

FROM alpine:3.19 AS builder

RUN apk add --no-cache \
    build-base autoconf flex bison \
    readline-dev ncurses-dev linux-headers \
    libssh-dev

COPY . /src/bird
WORKDIR /src/bird

RUN autoreconf -fi && \
    ./configure \
        --prefix=/usr \
        --sysconfdir=/etc/bird \
        --localstatedir=/var/run \
        --enable-ipv6 \
        --with-protocols=bgp,ospf,bfd,static,direct,kernel,device,radv,rpki,l3vpn && \
    make -j$(nproc) && \
    make install

FROM alpine:3.19

RUN apk add --no-cache libssh readline && \
    addgroup -S -g 153 bird && \
    adduser -S -u 153 -G bird bird

COPY --from=builder /usr/sbin/bird /usr/sbin/bird
COPY --from=builder /usr/sbin/birdc /usr/sbin/birdc
COPY bird.conf /etc/bird/bird.conf

RUN chown -R bird:bird /etc/bird && \
    mkdir -p /var/run && \
    chown -R bird:bird /var/run

EXPOSE 179

HEALTHCHECK --interval=30s --timeout=10s --retries=5 --start-period=60s \
    CMD /usr/sbin/birdc "show route" > /dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/sbin/bird", "-f", "-c", "/etc/bird/bird.conf"]
