import Header from "@/app/components/header";
import Buttons from "@/app/components/buttons";
import BankList from "@/app/components/banklists";
import Cashback from "@/app/components/cashback";
import Subscriptions from "@/app/components/subscriptions";
import Groups from "@/app/components/groups";
import Navbar from "@/app/components/navbar";

export default function Home() {
  return (
    <div className="bg-background flex flex-col gap-4 px-2">
      <Header />
      <BankList />
      <Buttons />
      <Cashback />
      <Subscriptions />
      <Groups />
      <Navbar />
    </div>
  );
}
