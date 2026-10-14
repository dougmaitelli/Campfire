export function EmptyPage({ title }: { title: string }) {
  return (
    <section className="page" aria-labelledby="page-title">
      <header className="page-header">
        <h1 id="page-title">{title}</h1>
      </header>
      <div className="page-content" />
    </section>
  );
}
