import { useRef, useState } from "react";
import { createSparkEnrollment } from "../api/sparks";
import type { Spark, SparkEnrollment } from "../api/sparks";
import { Drawer } from "../components/layout/Drawer";

export function EnrollSparkDialog({
  spark,
  onClose,
}: {
  spark: Spark;
  onClose: () => void;
}) {
  const [enrollment, setEnrollment] = useState<SparkEnrollment | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const pending = useRef(false);
  async function generate() {
    if (pending.current) return;
    pending.current = true;
    setBusy(true);
    setError(null);
    try {
      setEnrollment(await createSparkEnrollment(spark.id));
    } catch (cause) {
      setError(
        cause instanceof Error
          ? cause.message
          : "Unable to create enrollment token.",
      );
    } finally {
      pending.current = false;
      setBusy(false);
    }
  }
  return (
    <Drawer
      title={`Enroll ${spark.name}`}
      description="Connect a remote Spark agent to this panel."
      busy={busy}
      onClose={onClose}
      footerLeft={
        <button
          type="button"
          className="button"
          disabled={busy}
          onClick={onClose}
        >
          Close
        </button>
      }
      footerRight={
        <button
          type="button"
          className="button button-primary"
          disabled={busy}
          onClick={() => {
            void generate();
          }}
        >
          {busy
            ? "Generating…"
            : enrollment
              ? "Generate new token"
              : "Generate token"}
        </button>
      }
    >
      <div className="drawer-content">
        {error && (
          <p className="error-message" role="alert">
            {error}
          </p>
        )}
        <p>
          Generate a one-time token, then use it when starting the Spark agent
          on the remote host. New tokens replace previously unused tokens for
          this Spark.
        </p>
        {enrollment && (
          <>
            <label htmlFor="enrollment-token">Enrollment token</label>
            <textarea
              id="enrollment-token"
              rows={3}
              readOnly
              value={enrollment.token}
              spellCheck={false}
            />
            <p>
              Expires {new Date(enrollment.expires_at).toLocaleString()}. Save
              the token before closing this drawer.
            </p>
            <p>
              Put the token in a private file with permissions 0600 on the
              remote host, then run:
            </p>
            <pre className="agent-command">
              {
                "./spark --panel https://YOUR_PANEL:8443 --token-file enrollment.token --data spark-data"
              }
            </pre>
            <p>
              For a private HTTPS certificate, add{" "}
              <code>--ca-file panel-ca.crt</code>. After enrollment, restart
              with <code>./spark --data spark-data</code>.
            </p>
            <p>Refresh the Sparks page to see the connection status.</p>
          </>
        )}
      </div>
    </Drawer>
  );
}
