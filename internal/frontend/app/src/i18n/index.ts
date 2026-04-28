import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import en from './translations/en.json';

i18n.use(LanguageDetector)
	.use(initReactI18next)
	.init({
		resources: {
			en: { translation: en }
		},
		fallbackLng: 'en',
		supportedLngs: ['en'],
		interpolation: {
			escapeValue: false
		},
		detection: {
			order: ['navigator', 'htmlTag'],
			caches: []
		}
	});

export default i18n;
