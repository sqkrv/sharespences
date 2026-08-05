// AD-03 Канонические категории + алиасы, read-only (seed-managed).
import { useFetch, type Alias, type Canonical } from "../api";
import { Card, ErrMsg, SeedBanner, Spinner, TableWrap, Td, Th } from "../ui";

export default function Canonicals() {
  const { data: cats, error: cErr } = useFetch<Canonical[]>("/api/canonical-categories");
  const { data: aliases, error: aErr } = useFetch<Alias[]>("/api/aliases");
  if (cErr || aErr) return <ErrMsg error={cErr ?? aErr} />;
  if (!cats || !aliases) return <Spinner />;
  const byCanonical = new Map<number, Alias[]>();
  for (const a of aliases) {
    const list = byCanonical.get(a.canonical_category_id) ?? [];
    list.push(a);
    byCanonical.set(a.canonical_category_id, list);
  }
  return (
    <>
      <SeedBanner what="Канонические категории и алиасы" />
      <Card title={`${cats.length} канонических · ${aliases.length} алиасов`}>
        <TableWrap>
          <thead>
            <tr>
              <Th></Th>
              <Th>Slug</Th>
              <Th>Название</Th>
              <Th>Алиасы (банк: сырой заголовок)</Th>
            </tr>
          </thead>
          <tbody>
            {cats.map((c) => (
              <tr key={c.id}>
                <Td>{c.emoji ?? ""}</Td>
                <Td className="font-mono text-xs text-tx3">{c.slug}</Td>
                <Td>{c.title_ru}</Td>
                <Td className="text-tx3">
                  {(byCanonical.get(c.id) ?? []).map((a) => `${a.bank_name}: «${a.raw_title}»`).join(" · ") || "—"}
                </Td>
              </tr>
            ))}
          </tbody>
        </TableWrap>
      </Card>
    </>
  );
}
