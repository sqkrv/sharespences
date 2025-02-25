export default function Header() {
    return (
      <header className="p-4 bg-white shadow-md flex flex-col items-center">
        <p className="text-gray-500 text-sm">Nickname</p>
        <h1 className="text-2xl font-bold">Ваш баланс</h1>
        <p className="text-3xl font-semibold text-blue-600">123 000 ₽</p>
      </header>
    );
  }