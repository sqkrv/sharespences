export default function Navbar() {
    return (
      <nav className="fixed bottom-0 left-0 right-0 w-full bg-white shadow-md p-4 flex justify-around font-sans">
        <a href="/" className="text-blue-500">Главная</a>
        <a href="/cashback" className="text-gray-500">Кешбек</a>
        <a href="/groups" className="text-gray-500">Группы</a>
        <a href="/services" className="text-gray-500">Сервисы</a>
        <a href="/history" className="text-gray-500">История</a>
      </nav>
    );
  }