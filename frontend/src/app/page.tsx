import Banklist from "@/components/banklists";
import Buttons from "@/components/buttons";
import Cashback from "@/components/cashback";
import Groups from "@/components/groups";
import Header from "@/components/header";
import Navbar from "@/components/navbar";
import Subscriptions from "@/components/subscriptions";


export default function Home() {
  return (
    <div className="bg-background flex flex-col gap-4 px-2 pb-24">
      <Header />
      <Banklist />
      <Buttons />
      <Cashback />
      <Subscriptions />
      <Groups />
      <Navbar />
    </div>
  );
}
