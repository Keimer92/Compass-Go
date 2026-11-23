<script lang="ts">
  // Mascota de Compass: una brujula con brazos. SVG puro + CSS, sin assets:
  // hereda el color del contenedor (currentColor) y se adapta a cualquier tema.
  type Mood = 'idle' | 'working' | 'success' | 'error'

  let { size = 22, mood = 'idle' as Mood }: { size?: number; mood?: Mood } = $props()
</script>

<svg class="mascot {mood}" viewBox="0 0 48 48" width={size} height={size} aria-hidden="true">
  <g class="body">
    <!-- Brazos: el grupo rota desde el hombro, la mano acompana. -->
    <g class="arm left">
      <path d="M10.4 26.4c-3.1 1-4.7 3-4.9 5.4" />
      <circle class="hand" cx="4.9" cy="33" r="2.1" />
    </g>
    <g class="arm right">
      <path d="M37.6 26.4c3.1 1 4.7 3 4.9 5.4" />
      <circle class="hand" cx="43.1" cy="33" r="2.1" />
    </g>

    <!-- Esfera y aguja. -->
    <circle class="ring" cx="24" cy="24" r="14.4" />
    <circle class="face" cx="24" cy="24" r="10.9" />
    <g class="needle">
      <path class="needle-tip" d="M24 15.4 26.3 23.8h-4.6Z" />
    </g>

    <!-- Cara. -->
    <circle class="eye" cx="18.7" cy="20.2" r="1.6" />
    <circle class="eye" cx="29.3" cy="20.2" r="1.6" />
    <path class="mouth" d="M20.9 27.5c1.9 1.7 4.3 1.7 6.2 0" />
    <path class="mouth-sad" d="M20.9 29.1c1.9-1.7 4.3-1.7 6.2 0" />
  </g>
</svg>

<style>
  .mascot {
    display: block;
    overflow: visible;
  }
  .ring {
    fill: none;
    stroke: currentColor;
    stroke-width: 2.6;
  }
  .face {
    fill: none;
    stroke: currentColor;
    stroke-width: 0.9;
    opacity: 0.32;
  }
  .arm path {
    fill: none;
    stroke: currentColor;
    stroke-width: 2.2;
    stroke-linecap: round;
  }
  .hand {
    fill: currentColor;
  }
  .needle-tip {
    fill: var(--red);
  }
  .eye {
    fill: currentColor;
  }
  .mouth,
  .mouth-sad {
    fill: none;
    stroke: currentColor;
    stroke-width: 1.5;
    stroke-linecap: round;
  }
  .mouth-sad {
    opacity: 0;
  }

  /* --- Animaciones base (reposo) --- */
  .body {
    transform-box: fill-box;
    transform-origin: center;
    animation: float 4.2s ease-in-out infinite;
  }
  .eye {
    transform-box: fill-box;
    transform-origin: center;
    animation: blink 5.6s ease-in-out infinite;
  }
  .needle {
    /* La aguja debe girar sobre el centro de la esfera, no sobre el centro de
       su propia caja (que queda mas arriba y la haria orbitar). */
    transform-box: view-box;
    transform-origin: 24px 24px;
  }
  .arm {
    transform-box: fill-box;
  }
  .arm.left {
    transform-origin: 100% 0%;
  }
  .arm.right {
    transform-origin: 0% 0%;
  }

  @keyframes float {
    0%,
    100% {
      transform: translateY(0);
    }
    50% {
      transform: translateY(-1.1px);
    }
  }
  @keyframes blink {
    0%,
    93%,
    100% {
      transform: scaleY(1);
    }
    96%,
    98% {
      transform: scaleY(0.15);
    }
  }

  /* --- Trabajando: aguja girando, brazos saludando --- */
  .working .needle {
    animation: spin 1.5s cubic-bezier(0.45, 0, 0.55, 1) infinite;
  }
  .working .body {
    animation: bob 0.85s ease-in-out infinite;
  }
  .working .arm.left {
    animation: waveLeft 0.75s ease-in-out infinite;
  }
  .working .arm.right {
    animation: waveRight 0.75s ease-in-out infinite 0.12s;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @keyframes bob {
    0%,
    100% {
      transform: translateY(0) rotate(0deg);
    }
    50% {
      transform: translateY(-1.6px) rotate(-1.5deg);
    }
  }
  @keyframes waveLeft {
    0%,
    100% {
      transform: rotate(0deg);
    }
    50% {
      transform: rotate(-18deg);
    }
  }
  @keyframes waveRight {
    0%,
    100% {
      transform: rotate(0deg);
    }
    50% {
      transform: rotate(18deg);
    }
  }

  /* --- Exito: salta --- */
  .success .body {
    animation: hop 0.55s ease-out 3;
  }
  @keyframes hop {
    0%,
    100% {
      transform: translateY(0);
    }
    40% {
      transform: translateY(-3.4px);
    }
  }

  /* --- Error: se tambalea y pone cara triste --- */
  .error .body {
    animation: wobble 0.5s ease-in-out 2;
  }
  .error .mouth {
    opacity: 0;
  }
  .error .mouth-sad {
    opacity: 1;
  }
  .error .ring {
    stroke: var(--red);
  }
  .error .needle-tip {
    fill: var(--red);
  }
  @keyframes wobble {
    0%,
    100% {
      transform: rotate(0deg);
    }
    25% {
      transform: rotate(-7deg);
    }
    75% {
      transform: rotate(7deg);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .body,
    .eye,
    .needle,
    .arm {
      animation: none !important;
    }
  }
</style>
