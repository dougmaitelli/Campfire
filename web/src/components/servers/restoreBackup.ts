import i18n from "../../i18n";
import {
  serverAction,
  serverStatus,
  type RuntimeStatus,
} from "../../api/runtime";
import {
  OperationState,
  RuntimeState,
  ServerAction,
} from "../../contracts/domains";

export type RestorePhase =
  | typeof ServerAction.Stop
  | typeof ServerAction.Restore
  | typeof ServerAction.Start;

export async function restoreBackup(
  serverID: string,
  archive: string,
  options: { stop: boolean; restart: boolean },
  signal: AbortSignal,
  onStatus: (status: RuntimeStatus) => void,
  onPhase: (phase: RestorePhase) => void,
): Promise<RuntimeStatus> {
  async function run(action: RestorePhase, expectedState: RuntimeState) {
    signal.throwIfAborted();
    onPhase(action);
    let status = await serverAction(
      serverID,
      action,
      action === ServerAction.Restore ? archive : undefined,
    );
    const operationID = status.operation_id;
    if (!operationID)
      throw new Error(i18n.t("serverBackups.restoreOperationUnconfirmed"));
    for (;;) {
      signal.throwIfAborted();
      onStatus(status);
      if (status.operation_id !== operationID)
        throw new Error(i18n.t("serverBackups.restoreOperationChanged"));
      if (
        status.error ||
        status.error_code ||
        status.operation_state === OperationState.Failed ||
        status.operation_state === OperationState.Interrupted
      )
        throw new Error(
          status.error ?? i18n.t("serverBackups.restoreOperationFailed"),
        );
      if (!status.busy) {
        if (
          status.operation_state !== OperationState.Succeeded ||
          status.state !== expectedState
        )
          throw new Error(i18n.t("serverBackups.restoreOperationUnconfirmed"));
        return status;
      }
      await new Promise((resolve) => setTimeout(resolve, 1000));
      signal.throwIfAborted();
      status = await serverStatus(
        serverID,
        AbortSignal.any([signal, AbortSignal.timeout(12000)]),
      );
    }
  }

  signal.throwIfAborted();
  const initial = await serverStatus(
    serverID,
    AbortSignal.any([signal, AbortSignal.timeout(12000)]),
  );
  signal.throwIfAborted();
  if (initial.busy) throw new Error(i18n.t("serverBackups.restoreServerBusy"));
  const running = initial.state === RuntimeState.Running;
  if (running && !options.stop)
    throw new Error(i18n.t("serverBackups.restoreStopNotConfirmed"));
  if (
    !running &&
    initial.state !== RuntimeState.Stopped &&
    initial.state !== RuntimeState.NotInstalled
  )
    throw new Error(i18n.t("serverBackups.restoreServerBusy"));
  if (running) await run(ServerAction.Stop, RuntimeState.Stopped);
  const restored = await run(ServerAction.Restore, RuntimeState.Stopped);
  return running && options.restart
    ? run(ServerAction.Start, RuntimeState.Running)
    : restored;
}
