import type { Metadata } from "next";
import { headers } from "next/headers";
import { DICTIONARIES, langFromAcceptLanguage } from "@/lib/i18n";
import LandingClient from "./landing-client";

export async function generateMetadata(): Promise<Metadata> {
  const accept = (await headers()).get("accept-language");
  const lang = langFromAcceptLanguage(accept);
  const { meta } = DICTIONARIES[lang];
  return { title: meta.title, description: meta.description };
}

export default function LandingPage() {
  return <LandingClient />;
}
