import Header from "@/components/header";
import Navbar from "@/components/navbar";
import Groups from "@/components/groups/groups";
import ButtonsGroups from "@/components/groups/buttonsgroups";
import Waitlist from "@/components/groups/waitlist";
import ContactListPage from "@/components/groups/contactlist.page";

const Home = () => {
  return (
    <div className="bg-background flex flex-col gap-4 px-2 pb-24">
      <Header />
      <Navbar />
      <ContactListPage /> {}
      <ButtonsGroups />
      <Groups />
      <Waitlist />
    </div>
  );
}

export default Home;