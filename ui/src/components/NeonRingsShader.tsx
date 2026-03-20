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

  mat2 rotate2d(float angle) {
    return mat2(cos(angle), -sin(angle), sin(angle), cos(angle));
  }

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

    // Gentle lava-lamp UV warping
    float phase = mag * 3.0;
    uv += vec2(sin(uv.y * 3.0 + phase), cos(uv.x * 3.0 + phase)) * 0.05;
    uv = rotate2d(phase * 0.1) * uv;

    float dist = length(uv);

    // Smooth directional bias — full 360, no hard cutoffs
    vec2 accelDir = vec2(right - left, up - down);
    float accelMag = length(accelDir);
    float dirBias = 1.0;
    if (accelMag > 0.001 && dist > 0.001) {
      float alignment = dot(uv / dist, accelDir / accelMag) * 0.5 + 0.5;
      dirBias = 0.5 + 0.5 * alignment;
    }

    // Ring radii from overall mag — full circles
    float reach = effectReach(mag);
    float intensity = 0.0;
    for (int i = 0; i < 7; i++) {
      float fi = float(i);
      // Base radius + organic time-driven drift (lava lamp)
      float baseRadius = (fi + 1.0) / 7.0 * reach;
      float drift = sin(t * 0.5 + fi * 0.8) * reach * 0.08;
      float ringRadius = baseRadius + drift;

      float d = abs(dist - ringRadius);
      d += sin(uv.x * 3.0 + uv.y * 3.0) * 0.02;
      d = abs(d);
      float ring = smoothstep(0.03, 0.0, d) + smoothstep(0.05, 0.0, d) * 0.5;

      // Directional brightness
      ring *= dirBias;
      intensity += ring;
    }

    float edgeFade = smoothstep(1.0, 0.85, max(abs(uv.x), abs(uv.y)));
    intensity *= edgeFade;

    vec3 color = gyrColor(dist);
    gl_FragColor = vec4(color * intensity, clamp(intensity, 0.0, 1.0));
  }
`;

export default createShaderViz("NeonRingsShader", fragmentShader);
