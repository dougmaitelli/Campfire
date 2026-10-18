import { PageHeader } from "../components/layout/PageHeader";
export function EmptyPage({ title }: { title: string }) {
  return (
    <section className="page" aria-labelledby="page-title">
      <PageHeader title={title} />
      <div className="page-content" />
    </section>
  );
}
