import Header from "@/components/header";
import Navbar from "@/components/navbar";
import Subscriptions from "@/components/subscriptions";


export default function Home() {
  return (
    <div className="bg-background flex flex-col gap-4 px-2 pb-24">
      <Header />
      <Navbar />
    </div>
  );
}