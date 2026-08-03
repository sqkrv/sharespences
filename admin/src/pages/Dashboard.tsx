// AD-01 Дашборд: build, DB size, exact counts, goose status, table estimates.
import { useFetch, type Dashboard as Data } from "../api";
import { Card, ErrMsg, Spinner, TableWrap, Td, Th, Badge } from "../ui";

const COUNT_LABELS: [string, string][] = [
  ["banks", "Банки"],
  ["canonical_categories", "Канонические категории"],
  ["bank_categories", "Каталог банков"],
  ["custom_bank_categories", "— из них пользовательские"],
  ["aliases", "Алиасы"],
  ["programs", "Программы"],
  ["tiers", "Тарифы"],
  ["mcc_codes", "MCC-коды"],
  ["mcc_links", "Связки категория↔MCC"],
  ["mcc_changes", "Журнал MCC"],
  ["points_of_sale", "Точки продаж"],
  ["users", "Пользователи"],
  ["attachments", "Вложения"],
];

function mb(bytes: number) {
  return `${(bytes / 1024 / 1024).toFixed(1)} МБ`;
}

export default function Dashboard() {
  const { data, error } = useFetch<Data>("/api/dashboard");
  if (error) return <ErrMsg error={error} />;
  if (!data) return <Spinner />;
  const pending = data.migrations.filter((m) => !m.applied_at);
  return (
    <>
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-3">
        <Card title="База данных">
          <p className="text-2xl font-bold">{mb(data.db_size_bytes)}</p>
        </Card>
        <Card title="Миграции">
          <p className="text-2xl font-bold">
            {data.migrations.length - pending.length} / {data.migrations.length}
          </p>
          {pending.length > 0 && <Badge tone="warn">{pending.length} не применено</Badge>}
        </Card>
        <Card title="Точки продаж">
          <p className="text-2xl font-bold">{data.counts.points_of_sale?.toLocaleString("ru")}</p>
        </Card>
      </div>
      <Card title="Записи">
        <TableWrap>
          <tbody>
            {COUNT_LABELS.map(([key, label]) => (
              <tr key={key}>
                <Td className="text-tx2">{label}</Td>
                <Td className="text-right tabular-nums">{(data.counts[key] ?? 0).toLocaleString("ru")}</Td>
              </tr>
            ))}
          </tbody>
        </TableWrap>
      </Card>
      <Card title="Миграции (goose)">
        <TableWrap>
          <thead>
            <tr>
              <Th>Версия</Th>
              <Th>Файл</Th>
              <Th>Применена</Th>
            </tr>
          </thead>
          <tbody>
            {data.migrations.map((m) => (
              <tr key={m.version}>
                <Td className="tabular-nums">{m.version}</Td>
                <Td className="text-tx2">{m.source}</Td>
                <Td>{m.applied_at ?? <Badge tone="warn">ожидает</Badge>}</Td>
              </tr>
            ))}
          </tbody>
        </TableWrap>
      </Card>
      <Card title="Оценка строк по таблицам (pg_stat, включая незадействованные)">
        <TableWrap>
          <tbody>
            {data.tables.map((t) => (
              <tr key={t.table}>
                <Td className="text-tx2">{t.table}</Td>
                <Td className="text-right tabular-nums">{t.rows.toLocaleString("ru")}</Td>
              </tr>
            ))}
          </tbody>
        </TableWrap>
      </Card>
    </>
  );
}
