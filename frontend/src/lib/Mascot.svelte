<script lang="ts">
  // Mascota de Compass: una brujula 3D con brazos. SVG puro + CSS, sin assets:
  // hereda el color del contenedor (currentColor) y se adapta a cualquier tema.
  //
  // Rendimiento: el flotado y el balanceo viven en la envoltura HTML (transform
  // compuesto por GPU). Dentro del SVG solo se animan piezas pequenas (aguja,
  // brazos, ojos), de modo que la mascota no repinta el sidebar a 60 fps.
  type Mood = 'idle' | 'working' | 'success' | 'error'

  let { size = 40, mood = 'idle' as Mood, walk = false }: { size?: number; mood?: Mood; walk?: boolean } = $props()

  // IDs unicos por instancia: hay dos mascotas montadas (marca y pie).
  const uid = 'mz' + Math.random().toString(36).slice(2, 9)
</script>

<span class="mascot {mood}" class:walk style={`width:${size}px;height:${size}px`} aria-hidden="true">
  <span class="float">
    <svg viewBox="0 0 96 96" width="100%" height="100%">
      <defs>
        <radialGradient id={`bez-${uid}`} cx="30%" cy="22%" r="92%">
          <stop offset="0" stop-color="#fff" stop-opacity=".5" />
          <stop offset=".36" stop-color="#fff" stop-opacity=".06" />
          <stop offset=".66" stop-color="#000" stop-opacity="0" />
          <stop offset="1" stop-color="#000" stop-opacity=".5" />
        </radialGradient>
        <radialGradient id={`face-${uid}`} cx="42%" cy="30%" r="86%">
          <stop offset="0" stop-color="#34344e" />
          <stop offset=".55" stop-color="#191927" />
          <stop offset="1" stop-color="#0a0a11" />
        </radialGradient>
        <linearGradient id={`rim-${uid}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stop-color="#fff" stop-opacity=".55" />
          <stop offset=".5" stop-color="#fff" stop-opacity="0" />
          <stop offset="1" stop-color="#000" stop-opacity=".5" />
        </linearGradient>
        <linearGradient id={`needle-${uid}`} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0" stop-color="#ff9db4" />
          <stop offset=".55" stop-color="#f38ba8" />
          <stop offset="1" stop-color="#a83252" />
        </linearGradient>
        <radialGradient id={`gloss-${uid}`} cx="50%" cy="50%" r="50%">
          <stop offset="0" stop-color="#fff" stop-opacity=".5" />
          <stop offset="1" stop-color="#fff" stop-opacity="0" />
        </radialGradient>
        <radialGradient id={`hub-${uid}`} cx="35%" cy="30%" r="80%">
          <stop offset="0" stop-color="#fff" />
          <stop offset=".6" stop-color="#c9c9de" />
          <stop offset="1" stop-color="#6d6d88" />
        </radialGradient>
      </defs>

      <!-- Brazos: pivote en el hombro; la mano acompana al grupo. -->
      <g class="arm left">
        <path class="arm-base" d="M22 50c-8 3-12 8-13 15" />
        <path class="arm-shine" d="M22 50c-8 3-12 8-13 15" />
        <circle class="hand" cx="8.6" cy="68" r="4.6" />
        <ellipse class="hand-gloss" cx="7.2" cy="66.4" rx="1.7" ry="1.2" />
      </g>
      <g class="arm right">
        <path class="arm-base" d="M74 50c8 3 12 8 13 15" />
        <path class="arm-shine" d="M74 50c8 3 12 8 13 15" />
        <circle class="hand" cx="87.4" cy="68" r="4.6" />
        <ellipse class="hand-gloss" cx="86" cy="66.4" rx="1.7" ry="1.2" />
      </g>

      <!-- Cuerpo metalico en 3D: base currentColor + sombra/brillo encima. -->
      <circle class="bezel" cx="48" cy="45" r="31" />
      <circle class="bezel-shade" cx="48" cy="45" r="31" fill={`url(#bez-${uid})`} />
      <circle class="bezel-rim" cx="48" cy="45" r="31" fill="none" stroke={`url(#rim-${uid})`} />
      <circle class="face" cx="48" cy="45" r="23.4" fill={`url(#face-${uid})`} />
      <circle class="face-rim" cx="48" cy="45" r="23.4" fill="none" />

      <!-- Marcas cardinales. -->
      <g class="ticks">
        <path d="M48 24.2v3.2M48 62.6v3.2M27.2 45h3.2M65.6 45h3.2" />
      </g>

      <!-- Aguja: gira sobre el centro de la esfera. -->
      <g class="needle">
        <path class="needle-s" d="M48 66 53.4 45H42.6Z" />
        <path class="needle-n" d="M48 24 53.4 45H42.6Z" fill={`url(#needle-${uid})`} />
        <circle class="hub" cx="48" cy="45" r="3.6" fill={`url(#hub-${uid})`} />
        <circle class="hub-dot" cx="48" cy="45" r="1.1" />
      </g>

      <!-- Reflejo de cristal. -->
      <ellipse class="gloss" cx="37.5" cy="30.5" rx="11" ry="6.2" transform="rotate(-27 37.5 30.5)" fill={`url(#gloss-${uid})`} />

      <!-- Cara. -->
      <g class="eyes">
        <g class="eye">
          <circle class="eyeball" cx="38.4" cy="40.6" r="3.9" />
          <g class="pupils">
            <circle class="pupil" cx="39.1" cy="41" r="1.8" />
            <circle class="glint" cx="38.2" cy="39.9" r=".8" />
          </g>
        </g>
        <g class="eye">
          <circle class="eyeball" cx="57.6" cy="40.6" r="3.9" />
          <g class="pupils">
            <circle class="pupil" cx="58.3" cy="41" r="1.8" />
            <circle class="glint" cx="57.4" cy="39.9" r=".8" />
          </g>
        </g>
      </g>
      <ellipse class="cheek" cx="32.6" cy="49.4" rx="4.1" ry="2.5" />
      <ellipse class="cheek" cx="63.4" cy="49.4" rx="4.1" ry="2.5" />
      <path class="mouth" d="M41.4 52.2c2.1 3.4 11.1 3.4 13.2 0" />
      <path class="mouth-sad" d="M41.4 55c2.1-2.8 11.1-2.8 13.2 0" />

      <!-- Detalles de animo. -->
      <g class="spark">
        <path d="M76 11l2 4.6 4.6 2-4.6 2-2 4.6-2-4.6-4.6-2 4.6-2Z" />
        <path d="M87 25l1.4 3.4 3.4 1.4-3.4 1.4-1.4 3.4-1.4-3.4-3.4-1.4 3.4-1.4Z" />
        <path d="M68 3.5l1 2.5 2.5 1-2.5 1-1 2.5-1-2.5-2.5-1 2.5-1Z" />
      </g>
      <path class="sweat" d="M79 31c0 0 3 4 3 6a3 3 0 0 1-6 0c0-2 3-6 3-6Z" />
    </svg>
  </span>
</span>

<style>
  .mascot {
    position: relative;
    display: inline-grid;
    place-items: center;
    flex-shrink: 0;
  }
  /* El color se hereda en reposo (acento en la marca, apagado en el pie) y
     cambia con el animo para que la mascota tambien comunique el estado. */
  .mascot.working {
    color: var(--accent);
  }
  .mascot.success {
    color: var(--green);
  }
  .mascot.error {
    color: var(--red);
  }
  .float {
    display: block;
    width: 100%;
    height: 100%;
    transform: translateZ(0);
    will-change: transform;
    animation: float 4.6s ease-in-out infinite;
  }
  svg {
    display: block;
    width: 100%;
    height: 100%;
    overflow: visible;
  }

  /* Sombra en el suelo: se queda quieta mientras el cuerpo flota. */
  .mascot::before {
    content: '';
    position: absolute;
    left: 25%;
    right: 25%;
    bottom: 15%;
    height: 8%;
    border-radius: 50%;
    background: radial-gradient(closest-side, rgba(0, 0, 0, 0.5), transparent 78%);
    filter: blur(1.5px);
    animation: shadow 4.6s ease-in-out infinite;
    pointer-events: none;
  }

  .bezel {
    fill: currentColor;
  }
  .bezel-rim {
    stroke-width: 1.5;
  }
  .face-rim {
    stroke: rgba(0, 0, 0, 0.55);
    stroke-width: 1.6;
  }
  .ticks path {
    stroke: #fff;
    stroke-width: 1.3;
    stroke-linecap: round;
    opacity: 0.32;
  }
  .arm-base {
    fill: none;
    stroke: currentColor;
    stroke-width: 7;
    stroke-linecap: round;
  }
  .arm-shine {
    fill: none;
    stroke: #fff;
    stroke-width: 1.8;
    stroke-linecap: round;
    opacity: 0.22;
    transform: translate(-0.7px, -0.9px);
  }
  .hand {
    fill: currentColor;
  }
  .hand-gloss {
    fill: #fff;
    opacity: 0.45;
  }
  .needle-n {
    stroke: rgba(0, 0, 0, 0.25);
    stroke-width: 0.6;
  }
  .needle-s {
    fill: #e8e8f4;
    opacity: 0.22;
  }
  .hub-dot {
    fill: #2b2b3d;
  }
  .eyeball {
    fill: #f7f7ff;
  }
  .pupil {
    fill: #171722;
  }
  .glint {
    fill: #fff;
  }
  .cheek {
    fill: currentColor;
    opacity: 0.22;
  }
  .mouth,
  .mouth-sad {
    fill: none;
    stroke: #f4f4ff;
    stroke-width: 1.9;
    stroke-linecap: round;
  }
  .mouth-sad {
    opacity: 0;
  }
  .spark {
    fill: #ffd479;
    opacity: 0;
  }
  .sweat {
    fill: #8fd3ff;
    opacity: 0;
  }

  /* --- Reposo: flota, parpadea, mira alrededor y la aguja se balancea. --- */
  .needle {
    transform-box: view-box;
    transform-origin: 48px 45px;
    animation: sway 7s ease-in-out infinite;
  }
  .eye {
    transform-box: fill-box;
    transform-origin: center;
    animation: blink 5.4s ease-in-out infinite;
  }
  .pupils {
    transform-box: view-box;
    animation: look 9s ease-in-out infinite;
  }
  .arm {
    transform-box: view-box;
  }
  .arm.left {
    transform-origin: 22px 50px;
    animation: armSwayL 5.6s ease-in-out infinite;
  }
  .arm.right {
    transform-origin: 74px 50px;
    animation: armSwayR 5.6s ease-in-out infinite;
  }
  .gloss {
    animation: glint 6s ease-in-out infinite;
  }

  @keyframes float {
    0%,
    100% {
      transform: translateY(0);
    }
    50% {
      transform: translateY(-4.5%);
    }
  }
  @keyframes shadow {
    0%,
    100% {
      transform: scale(1);
      opacity: 0.55;
    }
    50% {
      transform: scale(0.86);
      opacity: 0.32;
    }
  }
  @keyframes blink {
    0%,
    92%,
    100% {
      transform: scaleY(1);
    }
    95%,
    97% {
      transform: scaleY(0.12);
    }
  }
  @keyframes look {
    0%,
    30%,
    100% {
      transform: translateX(0);
    }
    42%,
    54% {
      transform: translateX(1.3px);
    }
    66%,
    78% {
      transform: translateX(-1.3px);
    }
  }
  @keyframes sway {
    0%,
    100% {
      transform: rotate(-6deg);
    }
    50% {
      transform: rotate(6deg);
    }
  }
  @keyframes armSwayL {
    0%,
    100% {
      transform: rotate(0deg);
    }
    50% {
      transform: rotate(-4deg);
    }
  }
  @keyframes armSwayR {
    0%,
    100% {
      transform: rotate(0deg);
    }
    50% {
      transform: rotate(4deg);
    }
  }
  @keyframes glint {
    0%,
    70%,
    100% {
      opacity: 1;
    }
    78%,
    84% {
      opacity: 0.35;
    }
  }

  /* --- Trabajando: aguja girando, brazos saludando, cuerpo botando. --- */
  .working .float {
    animation: bob 0.85s ease-in-out infinite;
  }
  .working .needle {
    animation: spin 1.35s linear infinite;
  }
  .working .arm.left {
    animation: waveL 0.55s ease-in-out infinite;
  }
  .working .arm.right {
    animation: waveR 0.55s ease-in-out infinite 0.08s;
  }
  .working .gloss {
    animation: glint 1.6s ease-in-out infinite;
  }
  .working .eye {
    animation: blink 2.8s ease-in-out infinite;
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
      transform: translateY(-3%) rotate(-1.6deg);
    }
  }
  @keyframes waveL {
    0%,
    100% {
      transform: rotate(0deg);
    }
    50% {
      transform: rotate(-26deg);
    }
  }
  @keyframes waveR {
    0%,
    100% {
      transform: rotate(0deg);
    }
    50% {
      transform: rotate(26deg);
    }
  }

  /* --- Caminando: la mascota del panel derecho da pasos y gira al llegar. --- */
  .walk .float {
    animation: walkBob 0.5s ease-in-out infinite;
  }
  .walk .arm.left {
    animation: walkArmL 0.5s ease-in-out infinite;
  }
  .walk .arm.right {
    animation: walkArmR 0.5s ease-in-out infinite;
  }
  .walk .needle {
    animation: sway 3.2s ease-in-out infinite;
  }
  @keyframes walkBob {
    0%,
    100% {
      transform: translateY(0) rotate(-1.5deg);
    }
    50% {
      transform: translateY(-6%) rotate(1.5deg);
    }
  }
  @keyframes walkArmL {
    0%,
    100% {
      transform: rotate(-16deg);
    }
    50% {
      transform: rotate(16deg);
    }
  }
  @keyframes walkArmR {
    0%,
    100% {
      transform: rotate(16deg);
    }
    50% {
      transform: rotate(-16deg);
    }
  }

  /* --- Exito: salta, gira la aguja y salen destellos. --- */
  .success .float {
    animation: hop 0.5s ease-out 3;
  }
  .success .needle {
    animation: spin 0.6s ease-out 2;
  }
  .success .spark {
    animation: sparkle 0.75s ease-out 3;
  }
  .success .cheek {
    opacity: 0.42;
  }
  @keyframes hop {
    0%,
    100% {
      transform: translateY(0);
    }
    38% {
      transform: translateY(-9%);
    }
    62% {
      transform: translateY(-2%);
    }
  }
  @keyframes sparkle {
    0% {
      opacity: 0;
      transform: scale(0.5);
    }
    35% {
      opacity: 1;
      transform: scale(1.1);
    }
    100% {
      opacity: 0;
      transform: scale(0.8);
    }
  }

  /* --- Error: se tambalea, cara triste y gota de sudor. --- */
  .error .float {
    animation: wobble 0.45s ease-in-out 2;
  }
  .error .needle {
    animation: droop 0.5s ease-out forwards;
  }
  .error .mouth {
    opacity: 0;
  }
  .error .mouth-sad {
    opacity: 1;
  }
  .error .bezel-rim {
    stroke: var(--red);
  }
  .error .sweat {
    animation: sweat 0.9s ease-in 2;
  }
  @keyframes wobble {
    0%,
    100% {
      transform: rotate(0deg);
    }
    25% {
      transform: rotate(-6deg);
    }
    75% {
      transform: rotate(6deg);
    }
  }
  @keyframes droop {
    to {
      transform: rotate(132deg);
    }
  }
  @keyframes sweat {
    0% {
      opacity: 0;
      transform: translateY(-2px);
    }
    30% {
      opacity: 1;
    }
    100% {
      opacity: 0;
      transform: translateY(7px);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .float,
    .needle,
    .eye,
    .pupils,
    .arm,
    .gloss,
    .spark,
    .sweat,
    .mascot::before {
      animation: none !important;
    }
  }
</style>
