import { createShaderViz } from "@/lib/shaderViz";

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

  float effectReach(float force) {
    return clamp(force / (forceRedG * 1.5), 0.0, 1.0) * 0.85;
  }

  vec3 gyrColor(float d) {
    float norm = clamp(d / 0.85, 0.0, 1.0);
    float yellowT = forceYellowG / (forceRedG * 1.5);
    float redT = forceRedG / (forceRedG * 1.5);
    if (norm < yellowT) return mix(vec3(0.0, 1.0, 0.0), vec3(1.0, 1.0, 0.0), norm / yellowT);
    if (norm < redT) return mix(vec3(1.0, 1.0, 0.0), vec3(1.0, 0.0, 0.0), (norm - yellowT) / (redT - yellowT));
    return vec3(1.0, 0.0, 0.0);
  }

  void main(void) {
    vec2 uv = (gl_FragCoord.xy * 2.0 - resolution.xy) / min(resolution.x, resolution.y);
    float mag = sqrt(right * right + left * left + up * up + down * down);
    float t = time * 0.1;
    float dist = length(uv);

    // Smooth directional wave amplitude — full 360, no hard cutoffs
    vec2 accelDir = vec2(right - left, up - down);
    float accelMag = length(accelDir);
    float waveAmp = 1.0;
    if (accelMag > 0.001 && dist > 0.001) {
      float alignment = dot(uv / dist, accelDir / accelMag) * 0.5 + 0.5;
      waveAmp = 0.5 + 0.5 * alignment;
    }

    // Ring radii from overall mag — full circles
    float reach = effectReach(mag);

    // Expanding ripples from center
    float intensity = 0.0;
    for (int i = 0; i < 5; i++) {
      float fi = float(i);
      float phase = fract(t * 0.3 + fi * 0.2);
      float radius = phase * reach * 2.0;
      float age = phase;

      float width = 0.03 + (1.0 - age) * 0.04;
      float ring = smoothstep(width, 0.0, abs(dist - radius));
      ring *= (1.0 - age) * (1.0 - age);

      // Directional wave amplitude
      ring *= waveAmp;
      intensity += ring;
    }

    // Center core glow
    intensity += smoothstep(0.12, 0.0, dist) * clamp(mag / forceRedG, 0.0, 1.0);

    vec3 color = gyrColor(dist);
    gl_FragColor = vec4(color * intensity, clamp(intensity, 0.0, 1.0));
  }
`;

export default createShaderViz("ImpactRippleShader", fragmentShader);
