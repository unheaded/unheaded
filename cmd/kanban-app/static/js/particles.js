/**
 * particles.js - Particle Animation
 * Animated background with floating particles and connections
 * Matching bellis.tech aesthetic (dark #0a0a0a, accent #4a9eff)
 */

(function() {
    'use strict';

    // Configuration
    const CONFIG = {
        particleCount: 80,
        particleMinSize: 1,
        particleMaxSize: 3,
        particleSpeed: 0.3,
        connectionDistance: 150,
        mouseRadius: 200,
        mouseRepel: false,
        colors: {
            particle: '#4a9eff',
            particleSecondary: '#2a5a99',
            connection: 'rgba(74, 158, 255, 0.1)',
            connectionHover: 'rgba(74, 158, 255, 0.3)'
        },
        fps: 60
    };

    // State
    let canvas = null;
    let ctx = null;
    let particles = [];
    let animationId = null;
    let mousePos = { x: null, y: null };
    let width = 0;
    let height = 0;
    let dpr = 1;
    let lastFrameTime = 0;

    // Particle class
    class Particle {
        constructor() {
            this.reset();
        }

        reset() {
            this.x = Math.random() * width;
            this.y = Math.random() * height;
            this.size = CONFIG.particleMinSize + Math.random() * (CONFIG.particleMaxSize - CONFIG.particleMinSize);
            this.speedX = (Math.random() - 0.5) * CONFIG.particleSpeed;
            this.speedY = (Math.random() - 0.5) * CONFIG.particleSpeed;
            this.opacity = 0.3 + Math.random() * 0.5;
            this.pulseSpeed = 0.01 + Math.random() * 0.02;
            this.pulsePhase = Math.random() * Math.PI * 2;
            this.isGold = Math.random() > 0.3;
        }

        update(deltaTime) {
            // Update position
            this.x += this.speedX * deltaTime;
            this.y += this.speedY * deltaTime;

            // Pulse opacity
            this.pulsePhase += this.pulseSpeed * deltaTime;
            const pulse = Math.sin(this.pulsePhase) * 0.2;
            this.currentOpacity = Math.max(0.1, Math.min(1, this.opacity + pulse));

            // Mouse interaction
            if (mousePos.x !== null && mousePos.y !== null) {
                const dx = this.x - mousePos.x;
                const dy = this.y - mousePos.y;
                const distance = Math.sqrt(dx * dx + dy * dy);

                if (distance < CONFIG.mouseRadius) {
                    const force = (CONFIG.mouseRadius - distance) / CONFIG.mouseRadius;
                    if (CONFIG.mouseRepel) {
                        this.x += (dx / distance) * force * 2;
                        this.y += (dy / distance) * force * 2;
                    } else {
                        // Gentle attraction
                        this.x -= (dx / distance) * force * 0.5;
                        this.y -= (dy / distance) * force * 0.5;
                    }
                    // Brighten near mouse
                    this.currentOpacity = Math.min(1, this.currentOpacity + force * 0.3);
                }
            }

            // Wrap around edges
            if (this.x < -10) this.x = width + 10;
            if (this.x > width + 10) this.x = -10;
            if (this.y < -10) this.y = height + 10;
            if (this.y > height + 10) this.y = -10;
        }

        draw(ctx) {
            const color = this.isGold ? CONFIG.colors.particle : CONFIG.colors.particleSecondary;

            // Draw glow
            const gradient = ctx.createRadialGradient(
                this.x, this.y, 0,
                this.x, this.y, this.size * 4
            );
            gradient.addColorStop(0, color.replace(')', `, ${this.currentOpacity * 0.5})`).replace('rgb', 'rgba').replace('#4a9eff', 'rgba(74, 158, 255').replace('#2a5a99', 'rgba(42, 90, 153'));
            gradient.addColorStop(1, 'transparent');

            ctx.beginPath();
            ctx.arc(this.x, this.y, this.size * 4, 0, Math.PI * 2);
            ctx.fillStyle = `rgba(74, 158, 255, ${this.currentOpacity * 0.1})`;
            ctx.fill();

            // Draw particle
            ctx.beginPath();
            ctx.arc(this.x, this.y, this.size, 0, Math.PI * 2);
            ctx.fillStyle = this.isGold
                ? `rgba(74, 158, 255, ${this.currentOpacity})`
                : `rgba(42, 90, 153, ${this.currentOpacity})`;
            ctx.fill();
        }
    }

    // Initialize
    function init() {
        canvas = document.getElementById('particle-canvas');
        if (!canvas) {
            // Auto-create canvas appended to body (avoids flex layout issues)
            canvas = document.createElement('canvas');
            canvas.id = 'particle-canvas';
            canvas.style.cssText = 'position:fixed;top:0;left:0;width:100vw;height:100vh;z-index:-1;pointer-events:none;';
            document.body.appendChild(canvas);
        }

        ctx = canvas.getContext('2d');
        dpr = window.devicePixelRatio || 1;

        // Resize handling
        resize();
        window.addEventListener('resize', debounce(resize, 100));

        // Mouse tracking
        document.addEventListener('mousemove', handleMouseMove);
        document.addEventListener('mouseleave', handleMouseLeave);

        // Create particles
        createParticles();

        // Start animation
        startAnimation();

        console.log('[Particles] Initialized with', CONFIG.particleCount, 'particles');
        return true;
    }

    function resize() {
        width = window.innerWidth;
        height = window.innerHeight;

        canvas.width = width * dpr;
        canvas.height = height * dpr;
        canvas.style.width = width + 'px';
        canvas.style.height = height + 'px';

        ctx.scale(dpr, dpr);

        // Redistribute particles if already created
        if (particles.length > 0) {
            particles.forEach(p => {
                if (p.x > width) p.x = Math.random() * width;
                if (p.y > height) p.y = Math.random() * height;
            });
        }
    }

    function createParticles() {
        particles = [];
        for (let i = 0; i < CONFIG.particleCount; i++) {
            particles.push(new Particle());
        }
    }

    function handleMouseMove(e) {
        mousePos.x = e.clientX;
        mousePos.y = e.clientY;
    }

    function handleMouseLeave() {
        mousePos.x = null;
        mousePos.y = null;
    }

    function startAnimation() {
        function animate(timestamp) {
            const deltaTime = Math.min((timestamp - lastFrameTime) / 16.67, 3);
            lastFrameTime = timestamp;

            update(deltaTime);
            render();

            animationId = requestAnimationFrame(animate);
        }

        animationId = requestAnimationFrame(animate);
    }

    function stopAnimation() {
        if (animationId) {
            cancelAnimationFrame(animationId);
            animationId = null;
        }
    }

    function update(deltaTime) {
        particles.forEach(p => p.update(deltaTime));
    }

    function render() {
        // Clear with transparent background (CSS handles the actual background)
        ctx.clearRect(0, 0, width, height);

        // Draw connections first (behind particles)
        drawConnections();

        // Draw particles
        particles.forEach(p => p.draw(ctx));
    }

    function drawConnections() {
        for (let i = 0; i < particles.length; i++) {
            for (let j = i + 1; j < particles.length; j++) {
                const p1 = particles[i];
                const p2 = particles[j];

                const dx = p1.x - p2.x;
                const dy = p1.y - p2.y;
                const distance = Math.sqrt(dx * dx + dy * dy);

                if (distance < CONFIG.connectionDistance) {
                    const opacity = (1 - distance / CONFIG.connectionDistance) * 0.3;

                    // Check if mouse is near this connection
                    let mouseNear = false;
                    if (mousePos.x !== null && mousePos.y !== null) {
                        const midX = (p1.x + p2.x) / 2;
                        const midY = (p1.y + p2.y) / 2;
                        const mouseDist = Math.sqrt(
                            Math.pow(mousePos.x - midX, 2) +
                            Math.pow(mousePos.y - midY, 2)
                        );
                        mouseNear = mouseDist < CONFIG.mouseRadius;
                    }

                    ctx.beginPath();
                    ctx.moveTo(p1.x, p1.y);
                    ctx.lineTo(p2.x, p2.y);
                    ctx.strokeStyle = mouseNear
                        ? `rgba(74, 158, 255, ${opacity * 2})`
                        : `rgba(74, 158, 255, ${opacity})`;
                    ctx.lineWidth = mouseNear ? 1.5 : 1;
                    ctx.stroke();
                }
            }
        }

        // Draw connections to mouse
        if (mousePos.x !== null && mousePos.y !== null) {
            particles.forEach(p => {
                const dx = p.x - mousePos.x;
                const dy = p.y - mousePos.y;
                const distance = Math.sqrt(dx * dx + dy * dy);

                if (distance < CONFIG.mouseRadius) {
                    const opacity = (1 - distance / CONFIG.mouseRadius) * 0.4;

                    ctx.beginPath();
                    ctx.moveTo(p.x, p.y);
                    ctx.lineTo(mousePos.x, mousePos.y);
                    ctx.strokeStyle = `rgba(74, 158, 255, ${opacity})`;
                    ctx.lineWidth = 1;
                    ctx.stroke();
                }
            });
        }
    }

    // Utility: Debounce function
    function debounce(func, wait) {
        let timeout;
        return function executedFunction(...args) {
            const later = () => {
                clearTimeout(timeout);
                func(...args);
            };
            clearTimeout(timeout);
            timeout = setTimeout(later, wait);
        };
    }

    // Public API
    window.ParticleAnimation = {
        init,
        stop: stopAnimation,
        start: startAnimation,
        setConfig: function(newConfig) {
            Object.assign(CONFIG, newConfig);
        },
        getConfig: function() {
            return { ...CONFIG };
        }
    };

    // Auto-init when DOM ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', init);
    } else {
        init();
    }

    console.log('particles.js loaded');
})();
