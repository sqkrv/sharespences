import Header from "@/components/header";
import Navbar from "@/components/navbar";
import Cashback from "@/components/cashback/cashback";


const Home = () => {
  return (
    <div className="bg-background flex flex-col gap-4 px-2 pb-24">
      <Header />
      <Navbar />
      <Cashback />
    </div>
  );
}

export default Home