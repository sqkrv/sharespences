"use client";
import { usePathname, useRouter } from "next/navigation";
import Icon from "./Icon";

const Navbar = () => {
  const router = useRouter();
  const pathname = usePathname();

  const items = [
    {
      name: "Главная",
      icon: "",
      href: "/",
    },
    {
      name: "Кешбек",
      icon: "",
      href: "/cashback",
    },
    {
      name: "Группы",
      icon: "",
      href: "/groups",
    },
    {
      name: "Сервисы",
      icon: "",
      href: "/services",
    },
    {
      name: "История",
      icon: "",
      href: "/history",
    },
  ]

  return (
    <nav className="fixed bottom-0 left-0 right-0 w-full bg-white shadow-md p-4 flex justify-around font-sans">
      {items.map((item, index) => (
        <div onClick={() => router.push(item.href)} key={index} className="flex flex-col items-center">
          <Icon name="history" className="w-4 h-4 text-font" />
          <span className="text-gray-500">{item.name}</span>
        </div>
      ))}
    </nav>
  );
}

export default Navbar