import ReactDOM from "react-dom/client";
import "./index.css";
import { createBrowserRouter, RouterProvider } from "react-router-dom";
import VizRoute from "./routes/viz";
import CrosshairRoute from "./routes/crosshair";
import ConfigRoute from "./routes/config";

const router = createBrowserRouter([
  {
    path: "/",
    element: <ConfigRoute />,
  },
  {
    path: "/canvas",
    element: <VizRoute />,
  },
  {
    path: "/crosshair",
    element: <CrosshairRoute />,
  },
]);

document.addEventListener("DOMContentLoaded", () => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <RouterProvider router={router} />,
  );
});
