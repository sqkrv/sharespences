"use client";
import { usePathname, useRouter } from "next/navigation";
import IconNames from "./Icon";
import Icon from "./Icon";

type NavItem = {
  name: string;
  icon: typeof IconNames;
  href: string;
};

const items = [
  {
    name: "Главная",
    icon: "home",
    href: "/",
  },
  {
    name: "Кешбек",
    icon: "savings",
    href: "/cashback",
  },
  {
    name: "Группы",
    icon: "groups",
    href: "/groups",
  },
  {
    name: "Сервисы",
    icon: "cards",
    href: "/services",
  },
  {
    name: "История",
    icon: "history",
    href: "/history",
  },
] as const;

const Navbar = () => {
  const router = useRouter();
  const pathname = usePathname();

  return (
    <nav className="fixed bottom-0 left-0 right-0 w-full bg-background shadow-md p-4 flex justify-around font-sans">
      {items.map((item, index) => (
        <div onClick={() => router.push(item.href)} key={index} className="flex flex-col items-center">
          <Icon name={item.icon} className="w-6 h-6 text-font" />
          <span className="text-gray-500 text-button">{item.name}</span>
        </div>
      ))}
    </nav>
  );
};

export default Navbar;