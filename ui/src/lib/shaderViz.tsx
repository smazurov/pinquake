import { useRef, useEffect, useCallback, useImperativeHandle, forwardRef } from "react";
import * as THREE from "three";
// Like crosshair's decayValue but with smooth attack too (no instant snap-up)
function smoothValue(display: number, target: number, attackS: number, decayS: number): number {
  const rate = target >= display ? attackS : decayS;
  if (rate === 0) return target;
  const alpha = Math.exp(-3 / (rate * 60));
  return display * alpha + target * (1 - alpha);
}

export interface ShaderVizHandle {
  pushSample: (x: number, y: number) => void;
}

export interface ShaderVizConfig {
  forceYellowG: number;
  forceRedG: number;
  decayS: number;
}

interface ShaderVizProps {
  width: number;
  height: number;
  config?: ShaderVizConfig;
}

const DEFAULT_CONFIG: ShaderVizConfig = {
  forceYellowG: 0.03,
  forceRedG: 0.10,
  decayS: 0.3,
};

const ATTACK_S = 0.08; // ~5 frames to rise — smooth but responsive

const vertexShader = `
  void main() {
    gl_Position = vec4(position, 1.0);
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

export interface ExperimentEntry {
  id: string;
  label: string;
  Component: ReturnType<typeof createShaderViz>;
}

export function createShaderViz(name: string, fragmentShader: string) {
  const Component = forwardRef<ShaderVizHandle, ShaderVizProps>(function ShaderViz(
    { width, height, config: configOverride },
    ref,
  ) {
    const containerRef = useRef<HTMLDivElement>(null);
    const sceneRef = useRef<SceneState | null>(null);

    const configRef = useRef<ShaderVizConfig>(DEFAULT_CONFIG);
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

      const renderer = new THREE.WebGLRenderer({ alpha: true, premultipliedAlpha: false });
      renderer.setClearColor(0x000000, 0);
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

        const decayS = configRef.current.decayS;
        display.right = smoothValue(display.right, target.right, ATTACK_S, decayS);
        display.left = smoothValue(display.left, target.left, ATTACK_S, decayS);
        display.up = smoothValue(display.up, target.up, ATTACK_S, decayS);
        display.down = smoothValue(display.down, target.down, ATTACK_S, decayS);

        const mag = Math.sqrt(
          display.right ** 2 + display.left ** 2 + display.up ** 2 + display.down ** 2,
        );
        const decayFactor = 1 / Math.max(decayS, 0.05);
        const forceYellow = configRef.current.forceYellowG;
        const timeSpeed = Math.max(mag - forceYellow * 0.5, 0) / forceYellow;
        time += 0.05 * timeSpeed * decayFactor * 30.0;
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

  Component.displayName = name;
  return Component;
}
