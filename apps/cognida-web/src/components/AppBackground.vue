<template>
  <div class="app-background">
    <!-- 背景图片 -->
    <div class="bg-image" :style="{ backgroundImage: `url('${image}')` }"></div>

    <!-- 渐变遮罩 -->
    <div class="bg-overlay" :class="`variant-${variant}`"></div>

    <!-- 噪点层 -->
    <div v-if="noise" class="bg-noise"></div>

    <!-- 动态粒子（可选） -->
    <div v-if="particles" class="bg-particles">
      <span
        v-for="i in particleCount"
        :key="i"
        class="particle"
        :style="getParticleStyle(i)"
      ></span>
    </div>

    <!-- 渐变光晕 -->
    <div v-if="glow" class="bg-glow">
      <span class="glow-orb orb-1"></span>
      <span class="glow-orb orb-2"></span>
      <span class="glow-orb orb-3"></span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

interface Props {
  image?: string
  variant?: 'dark' | 'darker' | 'light' | 'gradient' | 'glass'
  noise?: boolean
  particles?: boolean
  particleCount?: number
  glow?: boolean
  opacity?: number
}

const props = withDefaults(defineProps<Props>(), {
  image: new URL('@/img/wallhaven-d83m63_1920x1080.png', import.meta.url).href,
  variant: 'dark',
  noise: true,
  particles: false,
  particleCount: 20,
  glow: true,
  opacity: 1
})

const particleStyles = computed(() => {
  const styles: Record<number, any> = {}
  for (let i = 1; i <= props.particleCount; i++) {
    const size = Math.random() * 4 + 2
    styles[i] = {
      left: Math.random() * 100 + '%',
      top: Math.random() * 100 + '%',
      width: size + 'px',
      height: size + 'px',
      animationDelay: Math.random() * 5 + 's',
      animationDuration: (Math.random() * 10 + 10) + 's'
    }
  }
  return styles
})

function getParticleStyle(index: number) {
  return particleStyles.value[index] || {}
}
</script>

<style scoped>
/* ==================== 背景容器 ==================== */
.app-background {
  position: fixed;
  inset: 0;
  z-index: -1;
  overflow: hidden;
}

/* ==================== 背景图片 ==================== */
.bg-image {
  position: absolute;
  inset: -50px;
  background-size: cover;
  background-position: center;
  background-repeat: no-repeat;
  filter: blur(0px);
}

/* ==================== 遮罩层 ==================== */
.bg-overlay {
  position: absolute;
  inset: 0;
  transition: all var(--transition-slow);
}

/* Dark 变体 - 默认 */
.variant-dark {
  background: linear-gradient(
    135deg,
    rgba(15, 23, 42, 0.92) 0%,
    rgba(30, 27, 75, 0.88) 25%,
    rgba(49, 46, 129, 0.85) 50%,
    rgba(30, 27, 75, 0.88) 75%,
    rgba(15, 23, 42, 0.92) 100%
  );
}

/* Darker - 更暗 */
.variant-darker {
  background: linear-gradient(
    135deg,
    rgba(15, 23, 42, 0.97) 0%,
    rgba(15, 23, 42, 0.95) 50%,
    rgba(30, 27, 75, 0.93) 100%
  );
}

/* Light - 较亮 */
.variant-light {
  background: linear-gradient(
    135deg,
    rgba(15, 23, 42, 0.85) 0%,
    rgba(30, 27, 75, 0.8) 50%,
    rgba(49, 46, 129, 0.75) 100%
  );
}

/* Gradient - 渐变 */
.variant-gradient {
  background: linear-gradient(
    135deg,
    rgba(99, 102, 241, 0.8) 0%,
    rgba(168, 85, 247, 0.7) 50%,
    rgba(6, 182, 212, 0.6) 100%
  );
}

/* Glass - 玻璃感 */
.variant-glass {
  background: linear-gradient(
    135deg,
    rgba(255, 255, 255, 0.05) 0%,
    rgba(255, 255, 255, 0.1) 50%,
    rgba(255, 255, 255, 0.05) 100%
  );
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
}

/* ==================== 噪点层 ==================== */
.bg-noise {
  position: absolute;
  inset: 0;
  opacity: 0.03;
  background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='noise'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23noise)'/%3E%3C/svg%3E");
  pointer-events: none;
}

/* ==================== 粒子层 ==================== */
.bg-particles {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.particle {
  position: absolute;
  background: rgba(255, 255, 255, 0.6);
  border-radius: 50%;
  animation: floatParticle linear infinite;
}

@keyframes floatParticle {
  0% {
    transform: translateY(100vh) scale(0);
    opacity: 0;
  }
  10% {
    opacity: 1;
  }
  90% {
    opacity: 1;
  }
  100% {
    transform: translateY(-10vh) scale(1);
    opacity: 0;
  }
}

/* ==================== 光晕层 ==================== */
.bg-glow {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
}

.glow-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  animation: orbFloat 20s ease-in-out infinite;
}

.orb-1 {
  width: 600px;
  height: 600px;
  top: -200px;
  left: -200px;
  background: radial-gradient(circle, rgba(99, 102, 241, 0.4) 0%, transparent 70%);
  animation-delay: 0s;
}

.orb-2 {
  width: 500px;
  height: 500px;
  bottom: -150px;
  right: -150px;
  background: radial-gradient(circle, rgba(168, 85, 247, 0.3) 0%, transparent 70%);
  animation-delay: -7s;
}

.orb-3 {
  width: 400px;
  height: 400px;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  background: radial-gradient(circle, rgba(6, 182, 212, 0.2) 0%, transparent 70%);
  animation-delay: -14s;
}

@keyframes orbFloat {
  0%, 100% {
    transform: translate(0, 0) scale(1);
  }
  25% {
    transform: translate(50px, -50px) scale(1.1);
  }
  50% {
    transform: translate(-30px, 30px) scale(0.9);
  }
  75% {
    transform: translate(-50px, -30px) scale(1.05);
  }
}

/* ==================== 响应式 ==================== */
@media (max-width: 768px) {
  .bg-image {
    inset: -100px;
  }

  .orb-1,
  .orb-2,
  .orb-3 {
    width: 300px;
    height: 300px;
  }
}
</style>
