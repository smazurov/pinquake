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

    vec2 dir = vec2(right - left, up - down);
    float dirMag = length(dir);

    float field = 0.0;

    // Center blob scales with magnitude
    float centerSize = 0.06 * smoothstep(0.0, forceYellowG, mag);
    field += centerSize / (dot(uv, uv) + 0.001);

    // Directional balls using effectReach
    if (dirMag > 0.0001) {
      vec2 axis = dir / dirMag;
      float reach = effectReach(dirMag);
      for (int i = 1; i <= 4; i++) {
        float fi = float(i);
        vec2 pos = axis * (fi / 4.0 * reach);
        float radius = dirMag * 0.25 * (1.0 - fi * 0.15);
        field += radius / (dot(uv - pos, uv - pos) + 0.001);
      }
    }

    // Per-direction secondary blobs using effectReach
    if (right > 0.001) {
      vec2 pos = vec2(effectReach(right), 0.0);
      field += (right * 0.15) / (dot(uv - pos, uv - pos) + 0.001);
    }
    if (left > 0.001) {
      vec2 pos = vec2(-effectReach(left), 0.0);
      field += (left * 0.15) / (dot(uv - pos, uv - pos) + 0.001);
    }
    if (up > 0.001) {
      vec2 pos = vec2(0.0, effectReach(up));
      field += (up * 0.15) / (dot(uv - pos, uv - pos) + 0.001);
    }
    if (down > 0.001) {
      vec2 pos = vec2(0.0, -effectReach(down));
      field += (down * 0.15) / (dot(uv - pos, uv - pos) + 0.001);
    }

    float edgeFade = smoothstep(1.0, 0.85, max(abs(uv.x), abs(uv.y)));
    float surface = smoothstep(1.0, 3.0, field);
    float glow = field * 0.04;
    float intensity = (surface + glow) * edgeFade;

    vec3 color = gyrColor(length(uv));
    gl_FragColor = vec4(color * intensity, clamp(intensity, 0.0, 1.0));
  }
`;

export default createShaderViz("MetaballShader", fragmentShader);
