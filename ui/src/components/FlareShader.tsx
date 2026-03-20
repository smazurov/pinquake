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
    float dist = length(uv);

    // Smooth directional flare using dot product — no axis artifacts
    vec2 accelDir = vec2(right - left, up - down);
    float accelMag = length(accelDir);
    float reach = effectReach(accelMag);

    float intensity = 0.0;
    if (accelMag > 0.001 && dist > 0.001) {
      float alignment = dot(uv / dist, accelDir / accelMag) * 0.5 + 0.5;
      float dirBias = 0.5 + 0.5 * alignment;
      // Cut off beyond 115% of reach
      float fade = smoothstep(reach * 1.15, reach * 0.8, dist);
      intensity += dirBias * fade * accelMag * 40.0;
    }

    // Per-arm reach for independent rebound visibility
    float reachR = effectReach(right);
    float reachL = effectReach(left);
    float reachU = effectReach(up);
    float reachD = effectReach(down);

    if (dist > 0.001) {
      vec2 pixDir = uv / dist;
      float armR = smoothstep(-0.3, 1.0, pixDir.x) * smoothstep(reachR * 1.15, 0.0, dist) * right * 15.0;
      float armL = smoothstep(-0.3, 1.0, -pixDir.x) * smoothstep(reachL * 1.15, 0.0, dist) * left * 15.0;
      float armU = smoothstep(-0.3, 1.0, pixDir.y) * smoothstep(reachU * 1.15, 0.0, dist) * up * 15.0;
      float armD = smoothstep(-0.3, 1.0, -pixDir.y) * smoothstep(reachD * 1.15, 0.0, dist) * down * 15.0;
      intensity += armR + armL + armU + armD;
    }

    // Center core
    intensity += smoothstep(0.15, 0.0, dist) * mag * 8.0;

    vec3 color = gyrColor(dist);
    gl_FragColor = vec4(color * intensity, clamp(intensity, 0.0, 1.0));
  }
`;

export default createShaderViz("FlareShader", fragmentShader);
