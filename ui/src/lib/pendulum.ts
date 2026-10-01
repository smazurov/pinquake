const G = 9.81;

export interface PendulumParams {
  bobDistance: number; // meters (pivot to bob)
  dampingRatio: number; // ζ: 0 = free, 1 = critically damped
}

export interface PendulumState {
  theta: number; // angular displacement (rad)
  omega: number; // angular velocity (rad/s)
}

/**
 * Advance one axis of the pendulum by dt using symplectic Euler.
 *
 * Equation of motion (pendulum on accelerating platform):
 *   θ̈ = -(g/l)·sinθ − 2ζω₀·θ̇ − (a_ext/l)·cosθ
 *
 * where ω₀ = √(g/l), l = bobDistance, a_ext = lateral accel in m/s².
 */
export function stepPendulum(
  state: PendulumState,
  accelMs2: number,
  params: PendulumParams,
  dt: number,
): PendulumState {
  const l = params.bobDistance;
  const omega0 = Math.sqrt(G / l);
  const { theta, omega } = state;

  const thetaAcc =
    -(G / l) * Math.sin(theta) -
    2 * params.dampingRatio * omega0 * omega -
    (accelMs2 / l) * Math.cos(theta);

  // Symplectic Euler: update velocity first, then position with new velocity
  const newOmega = omega + thetaAcc * dt;
  const newTheta = theta + newOmega * dt;

  return { theta: newTheta, omega: newOmega };
}

export function createState(): PendulumState {
  return { theta: 0, omega: 0 };
}
