import type { Metadata } from "next";
import { Maven_Pro } from "next/font/google";
import "./globals.css";

const mavenPro = Maven_Pro({
  variable: "--font-maven-pro",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "SmartAttend",
  description: "Employee attendance administration",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${mavenPro.variable} h-full antialiased`}>
      <body className="min-h-full">{children}</body>
    </html>
  );
}
