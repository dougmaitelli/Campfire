import { useState } from "react";
import "./game-art.css";

export function GameArt({ src }: { src?: string | undefined }) {
  const [failedArt, setFailedArt] = useState<string>();
  if (!src || failedArt === src) return null;
  return (
    <img
      className="game-art"
      src={src}
      alt=""
      width={40}
      height={40}
      loading="lazy"
      decoding="async"
      referrerPolicy="no-referrer"
      onError={() => setFailedArt(src)}
    />
  );
}
