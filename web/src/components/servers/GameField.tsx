interface FieldLabel {
  name: string;
}

interface FieldReading extends FieldLabel {
  value: string;
  available?: boolean;
}

export function GameFieldReading({
  field,
  unavailable,
}: {
  field: FieldReading;
  unavailable: string;
}) {
  return (
    <div>
      <dt>{field.name}</dt>
      <dd>{field.available !== false ? field.value : unavailable}</dd>
    </div>
  );
}
