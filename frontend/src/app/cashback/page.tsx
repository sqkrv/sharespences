import Header from "@/components/header";
import Navbar from "@/components/navbar";
import Cashback from "@/components/cashback/cashback";
import SavedCashback from "@/components/cashback/savedcashback";
import ButtonsCashback from "@/components/cashback/buttonscashback";

const Home = () => {
  return (
    <div className="bg-background flex flex-col gap-4 px-2 pb-24">
      <Header />
      <Navbar />
      <ButtonsCashback />
      <Cashback />
      <SavedCashback />
    </div>
  );
}

export default Home