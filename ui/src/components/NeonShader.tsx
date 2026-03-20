import { useRef, useEffect, useCallback, useImperativeHandle, forwardRef } from "react";
import * as THREE from "three";
import { decayValue } from "@/lib/crosshair";

export interface NeonShaderHandle {
  pushSample: (x: number, y: number) => void;
}

export interface NeonShaderConfig {
  forceYellowG: number;
  forceRedG: number;
}

interface NeonShaderProps {
  width: number;
  height: number;
  config?: NeonShaderConfig;
}

const DEFAULT_CONFIG: NeonShaderConfig = {
  forceYellowG: 0.03,
  forceRedG: 0.10,
};

const DECAY_S = 0.3;

const vertexShader = `
  void main() {
    gl_Position = vec4(position, 1.0);
  }
`;

const fragmentShader = `
  precision highp float;
  uniform vec2 resolution;
  uniform float time;
  uniform float right;
  uniform float left;
  uniform float up;
  uniform float down;
  uniform float forceYellowG;
  uniform float forceRedG;

  mat2 rotate2d(float angle) {
    return mat2(cos(angle), -sin(angle), sin(angle), cos(angle));
  }

  float random(vec2 st) {
    return fract(sin(dot(st.xy, vec2(12.9898, 78.233))) * 43758.5453123);
  }

  vec3 hsv2rgb(vec3 c) {
    vec4 K = vec4(1.0, 2.0 / 3.0, 1.0 / 3.0, 3.0);
    vec3 p = abs(fract(c.xxx + K.xyz) * 6.0 - K.www);
    return c.z * mix(K.xxx, clamp(p - K.xxx, 0.0, 1.0), c.y);
  }

  // Shared: g/y/r color from magnitude and spatial position
  vec3 gyrColor(float mag, float dist, float maxDist) {
    float maxG = forceRedG * 1.5;
    float forceNorm = clamp(mag / maxG, 0.0, 1.0);
    float yellowT = forceYellowG / maxG;
    float redT = forceRedG / maxG;
    vec3 outerColor;
    if (forceNorm < yellowT) {
      outerColor = mix(vec3(0.0, 1.0, 0.0), vec3(1.0, 1.0, 0.0), forceNorm / yellowT);
    } else if (forceNorm < redT) {
      outerColor = mix(vec3(1.0, 1.0, 0.0), vec3(1.0, 0.0, 0.0), (forceNorm - yellowT) / (redT - yellowT));
    } else {
      outerColor = vec3(1.0, 0.0, 0.0);
    }
    float outerness = clamp(dist / max(maxDist, 0.001), 0.0, 1.0);
    return mix(vec3(0.0, 1.0, 0.0), outerColor, outerness * outerness);
  }

  // Q1: Neon Rings — original technique
  vec3 neonRings(vec2 uv, float t, float mag, float stretchX, float stretchY) {
    uv += vec2(sin(uv.y * 4.0 + t * 2.0), cos(uv.x * 4.0 + t * 2.0)) * 0.1;
    uv = rotate2d(t * 0.25) * uv;
    uv.x /= (1.0 + stretchX * 10.0);
    uv.y /= (1.0 + stretchY * 10.0);

    float intensity = 0.0;
    for (int i = 0; i < 7; i++) {
      float fi = float(i);
      float wave = sin(t * 2.0 + fi * 0.5) * 0.5 + 0.5;
      float d = abs(wave - length(uv) + sin(uv.x + uv.y) * 0.1);
      intensity += smoothstep(0.03, 0.0, d) + 0.015 / (d + 0.01);
    }
    return gyrColor(mag, length(uv), 1.0) * intensity;
  }

  // Q2: Plasma Aurora — overlapping sin/cos interference
  vec3 plasma(vec2 uv, float t, float mag, float stretchX, float stretchY) {
    uv.x /= (1.0 + stretchX * 8.0);
    uv.y /= (1.0 + stretchY * 8.0);

    float freq = 3.0 + mag * 20.0;
    float v = 0.0;
    v += sin(uv.x * freq + t * 3.0);
    v += sin((uv.y * freq + t * 2.0) * 0.7);
    v += sin((uv.x * freq * 0.7 + uv.y * freq * 0.5 + t * 1.5));
    v += sin(length(uv) * freq * 1.2 - t * 2.5);
    v *= 0.25;

    // Hue: 0.33 (green) -> 0.16 (yellow) -> 0.0 (red) based on force
    float maxG = forceRedG * 1.5;
    float forceNorm = clamp(mag / maxG, 0.0, 1.0);
    float hue = mix(0.33, 0.0, forceNorm);
    float brightness = clamp(mag / forceRedG, 0.0, 1.0);

    vec3 color = hsv2rgb(vec3(hue + v * 0.1, 0.8, (0.5 + v * 0.5) * brightness));
    return color;
  }

  // Q3: Radial Pulse — expanding waves with exponential falloff
  vec3 radialPulse(vec2 uv, float t, float mag, float stretchX, float stretchY) {
    uv.x /= (1.0 + stretchX * 10.0);
    uv.y /= (1.0 + stretchY * 10.0);
    float dist = length(uv);

    float brightness = clamp(mag / forceRedG, 0.0, 1.0);

    // 3 harmonic layers
    float wave1 = sin(dist * 12.0 - t * 4.0) * 0.5 + 0.5;
    wave1 *= exp(-dist * 2.0);
    float wave2 = sin(dist * 8.0 - t * 3.0) * 0.5 + 0.5;
    wave2 *= exp(-dist * 1.5);
    float wave3 = sin(dist * 5.0 - t * 5.0) * 0.5 + 0.5;
    wave3 *= exp(-dist * 1.0);

    float intensity = (wave1 * 0.5 + wave2 * 0.3 + wave3 * 0.2) * brightness;

    // Bright core
    intensity += 0.08 / (dist + 0.05) * brightness;

    return gyrColor(mag, dist, 1.2) * intensity;
  }

  // Q4: Interference Ripples — multiple wave sources
  vec3 ripples(vec2 uv, float t, float mag, float stretchX, float stretchY) {
    float brightness = clamp(mag / forceRedG, 0.0, 1.0);

    // 4 source points that spread with acceleration
    float spread = mag * 8.0;
    vec2 s1 = vec2(spread * 0.5, 0.0);
    vec2 s2 = vec2(-spread * 0.5, 0.0);
    vec2 s3 = vec2(0.0, spread * 0.5);
    vec2 s4 = vec2(0.0, -spread * 0.5);

    float freq = 10.0 + mag * 30.0;
    float w = 0.0;
    w += cos(length(uv - s1) * freq - t * 3.0);
    w += cos(length(uv - s2) * freq - t * 3.5);
    w += cos(length(uv - s3) * freq - t * 2.5);
    w += cos(length(uv - s4) * freq - t * 4.0);
    w *= 0.25;

    float intensity = (w * 0.5 + 0.5) * brightness;
    float dist = length(uv);
    intensity *= exp(-dist * 0.8);

    return gyrColor(mag, dist, 1.5) * intensity;
  }

  void main(void) {
    // Determine quadrant and remap UV to local -1..1
    vec2 pixel = gl_FragCoord.xy;
    float halfW = resolution.x * 0.5;
    float halfH = resolution.y * 0.5;

    int qx = pixel.x < halfW ? 0 : 1;
    int qy = pixel.y < halfH ? 0 : 1;

    vec2 localPixel = vec2(
      qx == 0 ? pixel.x : pixel.x - halfW,
      qy == 0 ? pixel.y : pixel.y - halfH
    );
    float minDim = min(halfW, halfH);
    vec2 uv = (localPixel * 2.0 - vec2(halfW, halfH)) / minDim;

    float mag = sqrt(right * right + left * left + up * up + down * down);
    float stretchX = max(right, left);
    float stretchY = max(up, down);
    float t = time * 0.1;

    vec3 color = vec3(0.0);

    if (qx == 0 && qy == 1) {
      color = neonRings(uv, t, mag, stretchX, stretchY);
    } else if (qx == 1 && qy == 1) {
      color = plasma(uv, t, mag, stretchX, stretchY);
    } else if (qx == 0 && qy == 0) {
      color = radialPulse(uv, t, mag, stretchX, stretchY);
    } else {
      color = ripples(uv, t, mag, stretchX, stretchY);
    }

    color += (random(uv + mag) - 0.5) * 0.05;
    gl_FragColor = vec4(color, 1.0);
  }
`;

interface Directions {
  right: number;
  left: number;
  up: number;
  down: number;
}

interface SceneState {
  renderer: THREE.WebGLRenderer;
  uniforms: {
    resolution: THREE.IUniform<THREE.Vector2>;
    time: THREE.IUniform<number>;
    right: THREE.IUniform<number>;
    left: THREE.IUniform<number>;
    up: THREE.IUniform<number>;
    down: THREE.IUniform<number>;
    forceYellowG: THREE.IUniform<number>;
    forceRedG: THREE.IUniform<number>;
  };
  animationId: number;
  geometry: THREE.PlaneGeometry;
  material: THREE.ShaderMaterial;
}

const NeonShader = forwardRef<NeonShaderHandle, NeonShaderProps>(function NeonShader(
  { width, height, config: configOverride },
  ref,
) {
  const containerRef = useRef<HTMLDivElement>(null);
  const sceneRef = useRef<SceneState | null>(null);

  const configRef = useRef<NeonShaderConfig>(DEFAULT_CONFIG);
  useEffect(() => {
    configRef.current = { ...DEFAULT_CONFIG, ...configOverride };
    if (sceneRef.current) {
      sceneRef.current.uniforms.forceYellowG.value = configRef.current.forceYellowG;
      sceneRef.current.uniforms.forceRedG.value = configRef.current.forceRedG;
    }
  }, [configOverride]);

  const targetRef = useRef<Directions>({ right: 0, left: 0, up: 0, down: 0 });
  const displayRef = useRef<Directions>({ right: 0, left: 0, up: 0, down: 0 });

  const pushSample = useCallback((x: number, y: number) => {
    targetRef.current = {
      right: Math.max(x, 0),
      left: Math.max(-x, 0),
      up: Math.max(y, 0),
      down: Math.max(-y, 0),
    };
  }, []);

  useImperativeHandle(ref, () => ({ pushSample }), [pushSample]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    const camera = new THREE.Camera();
    camera.position.z = 1;

    const scene = new THREE.Scene();
    const geometry = new THREE.PlaneGeometry(2, 2);

    const config = configRef.current;
    const uniforms = {
      resolution: { value: new THREE.Vector2() },
      time: { value: 0.0 },
      right: { value: 0.0 },
      left: { value: 0.0 },
      up: { value: 0.0 },
      down: { value: 0.0 },
      forceYellowG: { value: config.forceYellowG },
      forceRedG: { value: config.forceRedG },
    };

    const material = new THREE.ShaderMaterial({
      uniforms,
      vertexShader,
      fragmentShader,
    });

    scene.add(new THREE.Mesh(geometry, material));

    const renderer = new THREE.WebGLRenderer({ alpha: true });
    renderer.setPixelRatio(window.devicePixelRatio);
    renderer.setSize(width, height);
    uniforms.resolution.value.set(renderer.domElement.width, renderer.domElement.height);

    container.appendChild(renderer.domElement);

    let animationId = 0;
    let time = 0;
    const animate = () => {
      animationId = requestAnimationFrame(animate);

      const target = targetRef.current;
      const display = displayRef.current;

      display.right = decayValue(display.right, target.right, DECAY_S);
      display.left = decayValue(display.left, target.left, DECAY_S);
      display.up = decayValue(display.up, target.up, DECAY_S);
      display.down = decayValue(display.down, target.down, DECAY_S);

      // Accumulate time scaled by magnitude — rings animate when moving, freeze when still
      const mag = Math.sqrt(
        display.right ** 2 + display.left ** 2 + display.up ** 2 + display.down ** 2,
      );
      time += 0.05 * (mag / 0.01); // at ~0.01g, runs at original speed
      uniforms.time.value = time;

      uniforms.right.value = display.right;
      uniforms.left.value = display.left;
      uniforms.up.value = display.up;
      uniforms.down.value = display.down;

      renderer.render(scene, camera);
    };

    sceneRef.current = { renderer, uniforms, animationId, geometry, material };
    animate();

    return () => {
      cancelAnimationFrame(animationId);
      if (container.contains(renderer.domElement)) {
        container.removeChild(renderer.domElement);
      }
      renderer.dispose();
      geometry.dispose();
      material.dispose();
      sceneRef.current = null;
    };
  }, [width, height]);

  return (
    <div
      ref={containerRef}
      style={{ width, height, background: "transparent", overflow: "hidden" }}
    />
  );
});

export default NeonShader;
