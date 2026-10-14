import type { Spark } from "../../api/sparks";

const dateFormat = new Intl.DateTimeFormat(undefined, {
  dateStyle: "medium",
  timeStyle: "short",
});

export function SparksTable({ sparks }: { sparks: Spark[] }) {
  return (
    <div className="table-container">
      <table className="sparks-table">
        <caption className="visually-hidden">Registered Sparks</caption>
        <thead>
          <tr>
            <th scope="col">Name</th>
            <th scope="col">Type</th>
            <th scope="col">Added</th>
          </tr>
        </thead>
        <tbody>
          {sparks.map((spark) => (
            <tr key={spark.id}>
              <td>
                <span className="spark-name">{spark.name}</span>
                <span className="spark-id">{spark.id}</span>
              </td>
              <td>
                <span className="type-badge">
                  {spark.kind === "local" ? "Local" : "Remote"}
                </span>
              </td>
              <td>
                <time dateTime={spark.created_at}>
                  {dateFormat.format(new Date(spark.created_at))}
                </time>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
