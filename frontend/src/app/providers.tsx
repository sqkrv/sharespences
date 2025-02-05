"use client";
import ErrorSnackbar from "@/components/ErrorSnackbar";
import SuccessSnackbar from "@/components/SuccessSnackbar";
import { Provider } from "react-redux";
import { store } from "@/redux/store";
import { FC, PropsWithChildren } from "react";
import { SnackbarProvider } from "notistack";

const ClientProvider: FC<PropsWithChildren> = ({ children }) => {
  return (
    <SnackbarProvider
      anchorOrigin={{
        vertical: "top",
        horizontal: "left",
      }}
      maxSnack={1}
      Components={{ success: SuccessSnackbar, error: ErrorSnackbar }}
      autoHideDuration={2000}
    >
      <Provider store={store}>{children}</Provider>
    </SnackbarProvider>
  );
};

export default ClientProvider;
