import type { ExperimentEntry } from "./shaderViz";
import NeonRingsShader from "../components/NeonRingsShader";
import MetaballShader from "../components/MetaballShader";
import ImpactRippleShader from "../components/ImpactRippleShader";
import FlareShader from "../components/FlareShader";

const experimentRegistry: ExperimentEntry[] = [
  { id: "neon", label: "Neon Rings", Component: NeonRingsShader },
  { id: "metaball", label: "Metaball", Component: MetaballShader },
  { id: "ripple", label: "Impact Ripple", Component: ImpactRippleShader },
  { id: "flare", label: "Flare", Component: FlareShader },
];

export default experimentRegistry;
