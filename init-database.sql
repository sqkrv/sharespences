drop owned by sharespences cascade;

create extension if not exists postgis;

create type payment_system as enum ('visa', 'mastercard', 'mir', 'unionpay', 'american_express');
create type transaction_status as enum ('hold', 'success');
create type transaction_direction as enum ('expense', 'income');

create table public."user"
(
    id           uuid primary key                  default gen_random_uuid(),
    username     text                     not null,
    display_name text                     not null,
    email        text                     not null,
    created_at   timestamp with time zone not null default now()
);
create unique index user_username_key on "user" using btree (username);
create unique index user_email_key on "user" using btree (email);

create table public.attachment
(
    id         uuid primary key default gen_random_uuid(),
    filename   text not null,
    media_type text
);

create table public.bank
(
    id            serial primary key,
    name          text not null,
    icon_filename text
);

create table public.bank_card
(
    id             serial primary key,
    bank_id        smallint       not null,
    user_id        uuid           not null,
    last_4_digits  integer        not null,
    payment_system payment_system not null,
    image_filename text,
    foreign key (bank_id) references public.bank (id),
    foreign key (user_id) references public."user" (id)
);

create table public.cashback
(
    id      serial primary key,
    date    timestamp with time zone not null,
    bank_id smallint                 not null,
    foreign key (bank_id) references public.bank (id)
);

create table public.category
(
    id            serial primary key,
    bank_id       smallint not null,
    name          text     not null,
    icon_filename text     not null,
    description   text     not null,
    foreign key (bank_id) references public.bank (id)
);

create table public.mcc_code
(
    code        smallint primary key,
    name        text not null,
    description text
);

create table public.category_mcc
(
    category_id integer  not null,
    mcc_code    smallint not null,
    primary key (category_id, mcc_code),
    foreign key (category_id) references public.category (id),
    foreign key (mcc_code) references public.mcc_code (code)
);

create table public.article
(
    id    serial primary key,
    title text not null,
    text  text not null
);
create table public.subscription
(
    id   serial primary key,
    name text not null
);

create table public.transaction
(
    id                    uuid primary key default gen_random_uuid(),
    user_id               uuid                     not null,
    og_id                 text                     not null,
    timestamp             timestamp with time zone not null,
    title                 text                     not null,
    amount                double precision         not null,
    direction             transaction_direction    not null,
    bank_id               smallint,
    merchandiser_logo_url text,
    bank_comment          text,
    mcc_code              smallint,
    category_id           integer,
    loyalty_amount        double precision,
    status                transaction_status       not null,
    location              geometry(Point, 4326),
    bank_card_id          integer,
    subscription_id       integer,
    user_comment          text,
    foreign key (bank_card_id) references public.bank_card (id),
    foreign key (bank_id) references public.bank (id),
    foreign key (category_id) references public.category (id),
    foreign key (subscription_id) references public.subscription (id),
    foreign key (user_id) references public."user" (id)
);

create index idx_transaction_location on transaction using gist (location);

create table public.transaction_attachment
(
    transaction_id uuid not null,
    attachment_id  uuid not null,
    primary key (transaction_id, attachment_id),
    foreign key (attachment_id) references public.attachment (id),
    foreign key (transaction_id) references public.transaction (id)
);

create table public.receipt_position
(
    id             uuid primary key default gen_random_uuid(),
    transaction_id uuid             not null,
    name           text             not null,
    quantity       double precision not null,
    amount         integer          not null,
    foreign key (transaction_id) references public.transaction (id)
);

create table public.transaction_user
(
    transaction_id     uuid    not null,
    user_id            uuid    not null,
    position_id        uuid,
    equal_distribution boolean not null,
    primary key (transaction_id, user_id),
    foreign key (transaction_id) references public.transaction (id),
    foreign key (position_id) references public.receipt_position (id),
    foreign key (user_id) references public."user" (id)
);

create table public.passkey
(
    id         text primary key, -- Base64URL encoded CredentialID
    user_id    uuid not null,
    name       text not null,
    public_key text not null,    -- Base64URL encoded PublicKey
    foreign key (user_id) references public."user" (id)
);

comment on column public.passkey.id is 'Base64URL encoded CredentialID';

comment on column public.passkey.public_key is 'Base64URL encoded PublicKey';

create table public.subscription_member
(
    subscription_id integer                  not null,
    user_id         uuid                     not null,
    since           timestamp with time zone not null default now(),
    primary key (subscription_id, user_id),
    foreign key (subscription_id) references public.subscription (id),
    foreign key (user_id) references public."user" (id)
);


insert into public.bank (name)
values ('Альфа-Банк'),
       ('Т-Банк'),
       ('Сбербанк'),
       ('ВТБ'),
       ('Ozon банк'),
       ('Яндекс Пэй');

insert into public.mcc_code (code, name, description)
values  (742, 'Ветеринарные услуги', 'Лицензированные специалисты в основном занимающиеся практикой ветеринарии, стоматологии или хирургии для всех видов животных; таких как домашние животные (например, собаки, кошки, рыба), домашний скот и другие фермерские животные (например, рогатый скот, лошади, овцы, свиньи, козы, домашние птицы, пчелы) и экзотические животные.'),
        (763, 'Сельскохозяйственные кооперативы', 'Ассоциации и кооперативы, которые предоставляют услуги управления фермами или оказывают помощь в сельскохозяйственных операциях. Примерами таких услуг являются финансовая помощь, управление или полное содержание сельскохозяйственных культур, подготовка почвы, посадка и культивация, аэрофотосжигание и распыление, борьба с болезнями и насекомыми, борьба с сорняками и сбор урожая.
Для точек, которые предоставляют складские помещения и хранилища ферм, используется MCC 4225.'),
        (780, 'Услуги садоводства и ландшафтного дизайна', 'Ландшафтные архитекторы и другие поставщики услуг по ландшафтному планированию и дизайну. Кроме того, точки, которые предлагают различные услуги по уходу за газоном и садом, такие как посадка, удобрение, выкос, мульчирование, посев, опрыскивание и укладка дерна.'),
        (1520, 'Генеральные подрядчики – жилое и коммерческое строительство', 'Генеральные подрядчики, в основном занимающиеся строительством жилых и коммерческих зданий. Строительные услуги могут включать новое строительство, реконструкцию, ремонт, дополнения и изменения.'),
        (1711, 'Генеральные подрядчики по вентиляции, теплоснабжению и водопроводу', 'Специальные торговые подрядчики, которые работают с системами отопления, водопровода и вентиляции. Примерами их услуг являются: балансировка и испытания вентиляционных системы, установка дренажной системы, ремонт отопления, установка опрскивателей газонов, работа с холодильными и морозильными камерами, подключения и соединения канализационных сетей, солнечное отопление, установка поливочных систем и установка и обслуживание водяных насосов.'),
        (1731, 'Подрядчики по электричеству', 'Специальные торговые подрядчики, выполняющие работы по электричеству, такие как установка пожарной сигнализации, звукового, телекоммуникационного и телефонного оборудования.'),
        (1740, 'Изоляция, мозаика, штукатурные работы, каменная кладка, облицовка плиткой, кафелем', 'Специальные торговые подрядчики, выполняющие мозаичные работы, каменные работы и другие работы с камнем, такие как строительство камина, облицовка плиткой, простая и декоративная штукатурка и изоляция. Эти подрядчики также могут выполнять работы по кирпичной кладке, керамике и мрамору, мозаичные работы, акустические работы и работы с конструкциями из гипсокартона.'),
        (1750, 'Столярные работы', 'Специальные торговые подрядчики, которые выполняют столярные работы для строительных проектов, таких как сборка, обрамление, отделка, а также установка окон и дверей.'),
        (1761, 'Кровельные и сайдинговые работы, обработка листового металла', 'Специальные торговые подрядчики, которые выполняют кровельные, обшивочные работы и выполняют работы из листового металла, включая архитектурные работы, потолки и световые люки, установку воздуховодов и водостоков, а также напыление, покраску или покрытие крыши.'),
        (1771, 'Подрядчики бетонных работ', 'Специальные торговые подрядчики, выполняющие бетонные, цементные или асфальтовые работы, строят частные подъездные и пешеходные пути из всех материалов, заливают бетон для фундаментов, выполняют цементные работы и строят бетонные дворики и тротуары.'),
        (1799, 'Контрактные услуги – нигде более не классифицированные', 'Специальные торговые подрядчики, выполняющие строительные работы, не классифицированные в других категориях. Примеры включают в себя установку навеса, сантехнические услуги, строительство заборов, установку пожарной лестницы, помощь при переездах, замену домашних окон, установку гаражных ворот, установку напольного покрытия, декоративные металлические работы, строительство бассейнов, стекольные работы, бурение скважин, поклейку обоев, гидроизоляцию и сварку.'),
        (3101, 'Авиалинии, авиакомпании', null),
        (3102, 'Iberia', null),
        (3103, 'Garuda (Индонезия)', null),
        (3104, 'Авиалинии, авиакомпании', null),
        (3105, 'Авиалинии, авиакомпании', null),
        (3106, 'Braathens S.A.F.E. (Норвегия)', null),
        (3107, 'Авиалинии, авиакомпании', null),
        (3747, 'Clubcorp/club Resorts', null),
        (2741, 'Различные издательства/ печатное дело', 'Торговые точки, занимающиеся оптовой печатью, издательской деятельностью или переплетом книг. Примеры материалов, производимых такими точками, включают книги, периодические издания, журналы, карты и атласы, информационные бюллетени для бизнеса, каталоги, партитуры, образцы документов, технические руководства и документы, телефонные справочники и ежегодники.

Для оптовых распространителей книг, брошюр и учебных материалов используется MCC 5192.'),
        (2791, 'Набор текстов, изготовление печатных форм и связанные услуги', 'Торговые точки, которые осуществляют оптовый набор текста для полиграфии и изготавливают печатные формы для полиграфических целей. Примерами таких услуг являются набор рекламных текстов, набор фотографий, автоматизированный набор текста и цветоделение; изготовление позитивов и негативов, из которых изготавливаются офсетные литографические печатные формы; и гравировка или тиснение для печатных целей, таких как гравировка по дереву, резине, меди, стали или фотогравировка.

Для торговых точек, которые в основном предоставляют оптовые услуги печати, используется MCC 2741.'),
        (2842, 'Санитарная обработка, полировка и специализированная подготовка', 'Оптовые производители полиролей, чистящих растворов, дезинфицирующих средств, моющих средств для больниц и другие санитарные препараты. Продукты для продажи могут включать дезодоранты неличного назначения, окрашивание искусственной кожи и других материалов, средства для удаления воска, растворители для обезжиривания, химические средства для сухой чистки, средства для удаления ржавчины и пятен, а также чистящие растворы для стекла, краски или обоев.'),
        (3000, 'United Airlines', null),
        (3001, 'American Airlines', null),
        (3002, 'Pan American', null),
        (3003, 'Eurofly', null),
        (3004, 'Dragonair', null),
        (3005, 'British Airways', null),
        (3006, 'Japan Air Lines', null),
        (3007, 'Air France', null),
        (3008, 'Lufthansa', null),
        (3009, 'Air Canada', null),
        (3010, 'Royal Dutch Airlines', null),
        (3011, 'Аэрофлот', null),
        (3012, 'Qantas', null),
        (3013, 'Alitalia', null),
        (3014, 'Saudi Arabian Airlines', null),
        (3015, 'Swiss International Air Lines', null),
        (3016, 'Scandinavian Airline System', null),
        (3017, 'South African Airways', null),
        (3018, 'Varig (Бразилия)', null),
        (3019, 'Авиалинии, авиакомпании', null),
        (3020, 'Air India', null),
        (3021, 'Air Algerie', null),
        (3022, 'Philippine Airlines', null),
        (3023, 'Mexicana', null),
        (3024, 'Pakistan International', null),
        (3025, 'Air New Zealand Limited International', null),
        (3026, 'Emirates Airlines', null),
        (3027, 'Union de Transports Aeriens', null),
        (3028, 'Air Malta', null),
        (3029, 'SN Brussels Airlines', null),
        (3030, 'Aerolineas Argentinas', null),
        (3031, 'Olympic Airways', null),
        (3032, 'El Al', null),
        (3033, 'Ansett Airlines', null),
        (3034, 'Etihad Airways', null),
        (3035, 'Tap (Португалия)', null),
        (3036, 'VASP (Бразилия)', null),
        (3037, 'EgyptAir', null),
        (3038, 'Kuwait Airways', null),
        (3039, 'Avianca', null),
        (3040, 'Gulf Air (Бахрейн)', null),
        (3041, 'Balkan—Bulgarian Airlines', null),
        (3042, 'Finnair', null),
        (3043, 'Aer Lingus', null),
        (3044, 'Air Lanka', null),
        (3045, 'Nigeria Airways', null),
        (3046, 'Cruzeiro do Sul (Бразилия)', null),
        (3047, 'Turkish Airlines', null),
        (3048, 'Royal Air Maroc', null),
        (3049, 'Tunis Air', null),
        (3050, 'Icelandair', null),
        (3051, 'Austrian Airlines', null),
        (3052, 'LAN Airlines', null),
        (3053, 'AVIACO (Испания)', null),
        (3054, 'LADECO (Чили)', null),
        (3055, 'LAB (Боливия)', null),
        (3056, 'Jet Airways', null),
        (3057, 'Virgin America', null),
        (3058, 'Delta', null),
        (3059, 'DBA Airlines', null),
        (3060, 'Northwest Airlines', null),
        (3061, 'Continental', null),
        (3062, 'Hapag-Lloyd Express', null),
        (3063, 'U.S. Airways', null),
        (3064, 'Adria Airways', null),
        (3065, 'Air Inter', null),
        (3066, 'Southwest Airlines', null),
        (3067, 'Vanguard Airlines', null),
        (3068, 'Air Astana', null),
        (3069, 'Sun Country Airlines', null),
        (3071, 'Air British Columbia', null),
        (3072, 'Cebu Pacific', null),
        (3073, 'Air Cal', null),
        (3075, 'Singapore Airlines', null),
        (3076, 'Aeromexico', null),
        (3077, 'Thai Airways', null),
        (3078, 'China Airlines', null),
        (3079, 'Jetstar Airways', null),
        (3081, 'Авиалинии, авиакомпании', null),
        (3082, 'Korean Airlines', null),
        (3083, 'Air Afrique', null),
        (3084, 'Eva Airways', null),
        (3085, 'Midwest Express Airlines', null),
        (3086, 'Carnival Airlines', null),
        (3087, 'Metro Airlines', null),
        (3088, 'Croatia Air', null),
        (3089, 'Transaero', null),
        (3090, 'Uni Airways', null),
        (3092, 'Midway Airlines', null),
        (3093, 'Авиалинии, авиакомпании', null),
        (3094, 'Zambia Airways', null),
        (3095, 'Авиалинии, авиакомпании', null),
        (3096, 'Air Zimbabwe', null),
        (3097, 'Spanair', null),
        (3098, 'Asiana Airlines', null),
        (3099, 'Cathay Pacific', null),
        (3100, 'Malaysian Airline System', null),
        (3109, 'Авиалинии, авиакомпании', null),
        (3110, 'Авиалинии, авиакомпании', null),
        (3111, 'British Midland', null),
        (3112, 'Windward Island', null),
        (3113, 'Авиалинии, авиакомпании', null),
        (3114, 'Авиалинии, авиакомпании', null),
        (3115, 'Авиалинии, авиакомпании', null),
        (3116, 'Авиалинии, авиакомпании', null),
        (3117, 'Venezolana International de Aviacion', null),
        (3118, 'Авиалинии, авиакомпании', null),
        (3119, 'Авиалинии, авиакомпании', null),
        (3120, 'Авиалинии, авиакомпании', null),
        (3121, 'Авиалинии, авиакомпании', null),
        (3122, 'Авиалинии, авиакомпании', null),
        (3123, 'Авиалинии, авиакомпании', null),
        (3124, 'Авиалинии, авиакомпании', null),
        (3125, 'Tan Airlines', null),
        (3126, 'Авиалинии, авиакомпании', null),
        (3127, 'Taca International', null),
        (3128, 'Авиалинии, авиакомпании', null),
        (3129, 'Surinam Airways', null),
        (3130, 'Sunworld International Airways', null),
        (3131, 'VLM Airlines', null),
        (3132, 'Frontier Airlines', null),
        (3133, 'Авиалинии, авиакомпании', null),
        (3134, 'Авиалинии, авиакомпании', null),
        (3135, 'Авиалинии, авиакомпании', null),
        (3136, 'Qatar Airways Company W.L.L.', null),
        (3137, 'Авиалинии, авиакомпании', null),
        (3138, 'Авиалинии, авиакомпании', null),
        (3139, 'Авиалинии, авиакомпании', null),
        (3140, 'Авиалинии, авиакомпании', null),
        (3141, 'Авиалинии, авиакомпании', null),
        (3142, 'Авиалинии, авиакомпании', null),
        (3143, 'Авиалинии, авиакомпании', null),
        (3144, 'Virgin Atlantic', null),
        (3145, 'Авиалинии, авиакомпании', null),
        (3146, 'Luxair', null),
        (3147, 'Авиалинии, авиакомпании', null),
        (3148, 'Air Littoral, S.A.', null),
        (3150, 'Авиалинии, авиакомпании', null),
        (3151, 'Air Zaire', null),
        (3152, 'Авиалинии, авиакомпании', null),
        (3153, 'Авиалинии, авиакомпании', null),
        (3154, 'Авиалинии, авиакомпании', null),
        (3155, 'Авиалинии, авиакомпании', null),
        (3156, 'GO FLY Ltd.', null),
        (3157, 'Авиалинии, авиакомпании', null),
        (3158, 'Авиалинии, авиакомпании', null),
        (3159, 'Provincetown-Boston Airways', null),
        (3160, 'Авиалинии, авиакомпании', null),
        (3161, 'All Nippon Airways', null),
        (3162, 'Авиалинии, авиакомпании', null),
        (3163, 'Авиалинии, авиакомпании', null),
        (3164, 'Norontair', null),
        (3165, 'Авиалинии, авиакомпании', null),
        (3166, 'Авиалинии, авиакомпании', null),
        (3167, 'Aero Continente', null),
        (3168, 'Авиалинии, авиакомпании', null),
        (3169, 'Авиалинии, авиакомпании', null),
        (3170, 'Авиалинии, авиакомпании', null),
        (3171, 'Canadian Airlines', null),
        (3172, 'Nation Air', null),
        (3173, 'Авиалинии, авиакомпании', null),
        (3174, 'JetBlue Airways', null),
        (3175, 'Middle East Air', null),
        (3176, 'Авиалинии, авиакомпании', null),
        (3177, 'AirTran Airways', null),
        (3178, 'Mesa Air', null),
        (3179, 'Авиалинии, авиакомпании', null),
        (3180, 'Westjet Airlines', null),
        (3181, 'Malev Hungarian Airlines', null),
        (3182, 'LOT – Polish Airlines', null),
        (3183, 'Oman Aviation Services', null),
        (3184, 'LIAT', null),
        (3185, 'LAV Linea Aeropostal Venezolana', null),
        (3186, 'LAP Lineas Aereas Paraguayas', null),
        (3187, 'LACSA (Коста Рика)', null),
        (3188, 'Virgin Express', null),
        (3189, 'Авиалинии, авиакомпании', null),
        (3190, 'Jugoslav Air', null),
        (3191, 'Island Airlines', null),
        (3192, 'Авиалинии, авиакомпании', null),
        (3193, 'Indian Airlines', null),
        (3194, 'Авиалинии, авиакомпании', null),
        (3195, 'Авиалинии, авиакомпании', null),
        (3196, 'Hawaiian Air', null),
        (3197, 'Havasu Airlines', null),
        (3198, 'Авиалинии, авиакомпании', null),
        (3199, 'Servicios Aereos Militares', null),
        (3200, 'Guyana Airways', null),
        (3201, 'Авиалинии, авиакомпании', null),
        (3202, 'Авиалинии, авиакомпании', null),
        (3203, 'Авиалинии, авиакомпании', null),
        (3204, 'Freedom Airlines', null),
        (3205, 'Авиалинии, авиакомпании', null),
        (3206, 'China Eastern Airlines', null),
        (3207, 'Авиалинии, авиакомпании', null),
        (3208, 'Авиалинии, авиакомпании', null),
        (3209, 'Авиалинии, авиакомпании', null),
        (3210, 'Авиалинии, авиакомпании', null),
        (3211, 'Norwegian Air Shuttle', null),
        (3212, 'Dominicana de Aviacion', null),
        (3213, 'Braathens Regional Airlines', null),
        (3214, 'Авиалинии, авиакомпании', null),
        (3215, 'Авиалинии, авиакомпании', null),
        (3216, 'Авиалинии, авиакомпании', null),
        (3217, 'CSA Ceskoslovenske Aerolinie', null),
        (3218, 'Авиалинии, авиакомпании', null),
        (3219, 'Compania Panamena de Aviacion', null),
        (3220, 'Compania Faucett', null),
        (3221, 'Transportes Aeros Militares Ecuatorianos', null),
        (3222, 'Command Airways', null),
        (3223, 'Comair', null),
        (3224, 'Авиалинии, авиакомпании', null),
        (3225, 'Авиалинии, авиакомпании', null),
        (3226, 'Skyways', null),
        (3227, 'Авиалинии, авиакомпании', null),
        (3228, 'Cayman Airways', null),
        (3229, 'SAETA (Sociedad Ecuatorianas De Transportes Aereo)', null),
        (3230, 'Авиалинии, авиакомпании', null),
        (3231, 'SAHSA (Servicio Aero de Honduras)', null),
        (3232, 'Авиалинии, авиакомпании', null),
        (3233, 'Авиалинии, авиакомпании', null),
        (3234, 'Caribbean Airlines', null),
        (3235, 'Авиалинии, авиакомпании', null),
        (3236, 'Air Arabia Airline', null),
        (3237, 'Авиалинии, авиакомпании', null),
        (3238, 'Авиалинии, авиакомпании', null),
        (3239, 'Bar Harbor Airlines', null),
        (3240, 'Bahamasair', null),
        (3241, 'Aviateca (Гватемала)', null),
        (3242, 'Avensa', null),
        (3243, 'Austrian Air Service', null),
        (3244, 'Авиалинии, авиакомпании', null),
        (3245, 'EasyJet', null),
        (3246, 'Ryanair', null),
        (3247, 'Gol Airlines', null),
        (3248, 'Tam Airlines', null),
        (3249, 'Авиалинии, авиакомпании', null),
        (3250, 'Авиалинии, авиакомпании', null),
        (3251, 'Авиалинии, авиакомпании', null),
        (3252, 'ALM Antilean Airlines', null),
        (3253, 'America West', null),
        (3254, 'Trump Airline', null),
        (3256, 'Alaska Airlines Inc.', null),
        (3257, 'Авиалинии, авиакомпании', null),
        (3258, 'Авиалинии, авиакомпании', null),
        (3259, 'Авиалинии, авиакомпании', null),
        (3260, 'Spirit Airlines', null),
        (3261, 'Air China', null),
        (3262, 'Авиалинии, авиакомпании', null),
        (3263, 'Aero Servicio Carabobo', null),
        (3264, 'Авиалинии, авиакомпании', 'Код указан на сайте Правительства Аляски как Авиалинии'),
        (3265, 'Авиалинии, авиакомпании', null),
        (3266, 'Air Seychelles', null),
        (3267, 'Air Panama International', null),
        (3268, 'Авиалинии, авиакомпании', null),
        (3270, 'Авиалинии, авиакомпании', null),
        (3274, 'Авиалинии, авиакомпании', null),
        (3275, 'Авиалинии, авиакомпании', null),
        (3276, 'Авиалинии, авиакомпании', null),
        (3277, 'Авиалинии, авиакомпании', null),
        (3278, 'Авиалинии, авиакомпании', null),
        (3279, 'Авиалинии, авиакомпании', null),
        (3280, 'Air Jamaica', null),
        (3281, 'Авиалинии, авиакомпании', null),
        (3282, 'Air Djibouti', null),
        (3283, 'Авиалинии, авиакомпании', null),
        (3284, 'Авиалинии, авиакомпании', null),
        (3285, 'Aero Peru', null),
        (3286, 'Aero Nicaraguenses', null),
        (3287, 'Aero Coach Aviation', null),
        (3288, 'Авиалинии, авиакомпании', null),
        (3289, 'Авиалинии, авиакомпании', null),
        (3290, 'Авиалинии, авиакомпании', null),
        (3291, 'Авиалинии, авиакомпании', null),
        (3292, 'Cyprus Airways', null),
        (3293, 'Ecuatoriana', null),
        (3294, 'Ethiopian Airlines', null),
        (3295, 'Kenya Airways', null),
        (3296, 'Air Berlin', null),
        (3297, 'Tarom Romanian Air Transport', null),
        (3298, 'Air Mauritius', null),
        (3299, 'Wideroes Flyveselskap', null),
        (3300, 'Azul Air', null),
        (3301, 'Wizz Air', null),
        (3302, 'Flybe Air', null),
        (3351, ' Affiliated Auto Rental', null),
        (3352, ' American Intl Rent-a-car', null),
        (3353, ' Brooks Rent-a-car', null),
        (3354, ' Action Auto Rental', null),
        (3355, 'Агентства по аренде автомобилей', null),
        (3356, 'Агентства по аренде автомобилей', null),
        (3357, ' Hertz Rent-a-car', null),
        (3358, 'Агентства по аренде автомобилей', null),
        (3359, ' Payless Car Rental', null),
        (3360, ' Snappy Car Rental', null),
        (3361, ' Airways Rent-a-car', null),
        (3362, ' Altra Auto Rental', null),
        (3363, 'Агентства по аренде автомобилей', null),
        (3364, ' Agency Rent-a-car', null),
        (3365, 'Агентства по аренде автомобилей', null),
        (3366, ' Budget Rent-a-car', null),
        (3367, 'Агентства по аренде автомобилей', null),
        (3368, ' Holiday Rent-a-wreck', null),
        (3369, 'Агентства по аренде автомобилей', null),
        (3370, ' Rent-a-wreck', null),
        (3371, 'Агентства по аренде автомобилей', null),
        (3372, 'Агентства по аренде автомобилей', null),
        (3373, 'Агентства по аренде автомобилей', null),
        (3374, 'Агентства по аренде автомобилей', null),
        (3375, 'Агентства по аренде автомобилей', null),
        (3376, ' Ajax Rent-a-car', null),
        (3377, 'Агентства по аренде автомобилей', null),
        (3378, 'Агентства по аренде автомобилей', null),
        (3379, 'Агентства по аренде автомобилей', null),
        (3380, 'Агентства по аренде автомобилей', null),
        (3381, ' Europ Car', null),
        (3382, 'Агентства по аренде автомобилей', null),
        (3383, 'Агентства по аренде автомобилей', null),
        (3384, 'Агентства по аренде автомобилей', null),
        (3385, ' Tropical Rent-a-car', null),
        (3386, ' Showcase Rental Cars', null),
        (3387, ' Alamo Rent-a-car', null),
        (3388, 'Агентства по аренде автомобилей', null),
        (3389, ' Avis Rent-a-car', null),
        (3390, ' Dollar Rent-a-car', null),
        (3391, ' Europe By Car', null),
        (3392, 'Агентства по аренде автомобилей', null),
        (3393, ' National Car Rental', null),
        (3394, ' Kemwell Group Rent-a-car', null),
        (3395, ' Thrifty Rent-a-car', null),
        (3396, ' Tilden Tent-a-car', null),
        (3397, 'Агентства по аренде автомобилей', null),
        (3398, ' Econo-car Rent-a-car', null),
        (3399, 'Amerex Rent-a-Car', null),
        (3400, ' Auto Host Cost Car Rentals', null),
        (3401, 'Агентства по аренде автомобилей', null),
        (3402, 'Агентства по аренде автомобилей', null),
        (3403, 'Агентства по аренде автомобилей', null),
        (3404, 'Агентства по аренде автомобилей', null),
        (3405, ' Enterprise Rent-a-car', null),
        (3406, 'Агентства по аренде автомобилей', null),
        (3407, 'Агентства по аренде автомобилей', null),
        (3408, 'Агентства по аренде автомобилей', null),
        (3409, ' General Rent-a-car', null),
        (3410, 'Агентства по аренде автомобилей', null),
        (3412, ' A-1 Rent-a-car', null),
        (3413, 'Агентства по аренде автомобилей', null),
        (3414, ' Godfrey Natl Rent-a-car', null),
        (3415, 'Агентства по аренде автомобилей', null),
        (3416, 'Агентства по аренде автомобилей', null),
        (3417, 'Агентства по аренде автомобилей', null),
        (3418, 'Агентства по аренде автомобилей', null),
        (3419, ' Alpha Rent-a-car', null),
        (3420, ' Ansa Intl Rent-a-car', null),
        (3421, ' Allstae Rent-a-car', null),
        (3422, 'Агентства по аренде автомобилей', null),
        (3423, ' Avcar Rent-a-car', null),
        (3425, ' Automate Rent-a-car', null),
        (3426, 'Агентства по аренде автомобилей', null),
        (3427, ' Avon Rent-a-car', null),
        (3428, ' Carey Rent-a-car', null),
        (3429, ' Insurance Rent-a-car', null),
        (3430, ' Major Rent-a-car', null),
        (3431, ' Replacement Rent-a-car', null),
        (3432, ' Reserve Rent-a-car', null),
        (3433, ' Ugly Duckling Rent-a-car', null),
        (3434, ' USA Rent-a-car', null),
        (3435, ' Value Rent-a-car', null),
        (3436, ' Autohansa Rent-a-car', null),
        (3437, ' Cite Rent-a-car', null),
        (3438, ' Interent Rent-a-car', null),
        (3439, ' Milleville Rent-a-car', null),
        (3440, 'Via Route Rent-a-Car', null),
        (3441, 'Агентства по аренде автомобилей', null),
        (3501, 'Holiday Inns', null),
        (3502, 'Best Western Hotels', null),
        (3503, 'Sheraton Hotels', null),
        (3504, 'Hilton Hotels', null),
        (3505, 'Forte Hotels', null),
        (3506, 'Golden Tulip Hotels', null),
        (3507, 'Friendship Inns', null),
        (3508, 'Quality Inns', null),
        (3509, 'Marriott Hotels', null),
        (3510, 'Days Inn', null),
        (3511, 'Arabella Hotels', null),
        (3512, 'Inter-continental Hotels', null),
        (3513, 'Westin Hotels', null),
        (3514, 'Отели, мотели, курорты', null),
        (3515, 'Rodeway Inns', null),
        (3516, 'La Quinta Motor Inns', null),
        (3517, 'Americana Hotels', null),
        (3518, 'Sol Hotels', null),
        (3519, 'Pullman International Hotels', null),
        (3520, 'Meridien Hotels', null),
        (3521, 'Crest Hotels (see Forte Hotels)', null),
        (3522, 'Tokyo Hotel', null),
        (3523, 'Pennsula Hotel', null),
        (3524, 'Welcomgroup Hotels', null),
        (3525, 'Dunfey Hotels', null),
        (3526, 'Отели, мотели, курорты', null),
        (3527, 'Downtowner-passport Hotel', null),
        (3528, 'Red Lion Hotels', null),
        (3529, 'Cp Hotels', null),
        (3530, 'Renaissance Hotels', null),
        (3531, 'Astir Hotels', null),
        (3532, 'Sun Route Hotels', null),
        (3533, 'Hotel Ibis', null),
        (3534, 'Southern Pacific Hotels', null),
        (3535, 'Hilton International', null),
        (3536, 'Amfac Hotels', null),
        (3537, 'Ana Hotel', null),
        (3538, 'Concorde Hotels', null),
        (3539, 'Отели, мотели, курорты', null),
        (3540, 'Iberotel Hotels', null),
        (3541, 'Hotel Okura', null),
        (3542, 'Royal Hotels', null),
        (3543, 'Four Seasons Hotels', null),
        (3544, 'Ciga Hotels', null),
        (3545, 'Shangri-la International', null),
        (3546, 'Отели, мотели, курорты', null),
        (3547, 'Отели, мотели, курорты', null),
        (3548, 'Hoteles Melia', null),
        (3549, 'Auberge Des Governeurs', null),
        (3550, 'Regal 8 Inns', null),
        (3551, 'Отели, мотели, курорты', null),
        (3552, 'Coast Hotels', null),
        (3553, 'Park Inns International', null),
        (3554, 'Отели, мотели, курорты', null),
        (3555, 'Отели, мотели, курорты', null),
        (3556, 'Отели, мотели, курорты', null),
        (3557, 'Отели, мотели, курорты', null),
        (3558, 'Jolly Hotels', null),
        (3559, 'Отели, мотели, курорты', null),
        (3560, 'Отели, мотели, курорты', null),
        (3561, 'Отели, мотели, курорты', null),
        (3562, 'Comfort Inns', null),
        (3563, 'Journey’s End Motls', null),
        (3564, 'Отели, мотели, курорты', null),
        (3565, 'Relax Inns', null),
        (3566, 'Отели, мотели, курорты', null),
        (3567, 'Отели, мотели, курорты', null),
        (3568, 'Ladbroke Hotels', null),
        (3569, 'Отели, мотели, курорты', null),
        (3570, 'Forum Hotels', null),
        (3571, 'Отели, мотели, курорты', null),
        (3572, 'Miyako Hotels', null),
        (3573, 'Sandman Hotels', null),
        (3574, 'Venture Inns', null),
        (3575, 'Vagabond Hotels', null),
        (3576, 'Отели, мотели, курорты', null),
        (3577, 'Mandarin Oriental Hotel', null),
        (3578, 'Отели, мотели, курорты', null),
        (3579, 'Hotel Mercure', null),
        (3580, 'Отели, мотели, курорты', null),
        (3581, 'Delta Hotel', null),
        (3582, 'Отели, мотели, курорты', null),
        (3583, 'Sas Hotels', null),
        (3584, 'Princess Hotels International', null),
        (3585, 'Hungar Hotels', null),
        (3586, 'Sokos Hotels', null),
        (3587, 'Doral Hotels', null),
        (3588, 'Helmsley Hotels', null),
        (3589, 'Отели, мотели, курорты', null),
        (3590, 'Fairmont Hotels', null),
        (3591, 'Sonesta Hotels', null),
        (3592, 'Omni Hotels', null),
        (3593, 'Cunard Hotels', null),
        (3594, 'Отели, мотели, курорты', null),
        (3595, 'Hospitality International', null),
        (3596, 'Отели, мотели, курорты', null),
        (3597, 'Отели, мотели, курорты', null),
        (3598, 'Regent International Hotels', null),
        (3599, 'Pannonia Hotels', null),
        (3600, 'Отели, мотели, курорты', null),
        (3601, 'Отели, мотели, курорты', null),
        (3602, 'Отели, мотели, курорты', null),
        (3603, 'Noah’s Hotels', null),
        (3604, 'Отели, мотели, курорты', null),
        (3605, 'Отели, мотели, курорты', null),
        (3606, 'Отели, мотели, курорты', null),
        (3607, 'Отели, мотели, курорты', null),
        (3608, 'Отели, мотели, курорты', null),
        (3609, 'Отели, мотели, курорты', null),
        (3610, 'Отели, мотели, курорты', null),
        (3611, 'Отели, мотели, курорты', null),
        (3612, 'Movenpick Hotels', null),
        (3613, 'Отели, мотели, курорты', null),
        (3614, 'Отели, мотели, курорты', null),
        (3615, 'Travelodge', null),
        (3616, 'Отели, мотели, курорты', null),
        (3617, 'Отели, мотели, курорты', null),
        (3618, 'Отели, мотели, курорты', null),
        (3619, 'Отели, мотели, курорты', null),
        (3620, 'Telford International', null),
        (3621, 'Отели, мотели, курорты', null),
        (3622, 'Merlin Hotels', null),
        (3623, 'Dorint Hotels', null),
        (3624, 'Отели, мотели, курорты', null),
        (3625, 'Hotle Universale', null),
        (3626, 'Prince Hotels', null),
        (3627, 'Отели, мотели, курорты', null),
        (3628, 'Отели, мотели, курорты', null),
        (3629, 'Dan Hotels', null),
        (3630, 'Отели, мотели, курорты', null),
        (3631, 'Отели, мотели, курорты', null),
        (3632, 'Отели, мотели, курорты', null),
        (3633, 'Rank Hotels', null),
        (3634, 'Swissotel', null),
        (3635, 'Reso Hotels', null),
        (3636, 'Sarova Hotels', null),
        (3637, 'Ramada Inns', null),
        (3638, 'Ho Jo Inn', null),
        (3639, 'Mount Charlotte Thistle', null),
        (3640, 'Hyatt Hotel', null),
        (3641, 'Sofitel Hotels', null),
        (3642, 'Novotel Hotels', null),
        (3643, 'Steigenberger Hotels', null),
        (3644, 'Econo Lodges', null),
        (3645, 'Queens Moat Houses', null),
        (3646, 'Swallow Hotels', null),
        (3647, 'Husa Hotels', null),
        (3648, 'De Vere Hotels', null),
        (3649, 'Radisson Hotels', null),
        (3650, 'Red Rook Inns', null),
        (3651, 'Imperial London Hotel', null),
        (3652, 'Embassy Hotels', null),
        (3653, 'Penta Hotels', null),
        (3654, 'Loews Hotels', null),
        (3655, 'Scandic Hotels', null),
        (3656, 'Sara Hotels', null),
        (3657, 'Oberoi Hotels', null),
        (3658, 'Otani Hotels', null),
        (3659, 'Taj Hotels International', null),
        (3660, 'Knights Inns', null),
        (3661, 'Metropole Hotels', null),
        (3662, 'Отели, мотели, курорты', null),
        (3663, 'Hoteles El Presidents', null),
        (3664, 'Flag Inn', null),
        (3665, 'Hampton Inns', null),
        (3666, 'Stakis Hotels', null),
        (3667, 'Отели, мотели, курорты', null),
        (3668, 'Maritim Hotels', null),
        (3669, 'Отели, мотели, курорты', null),
        (3670, 'Arcard Hotels', null),
        (3671, 'Arctia Hotels', null),
        (3672, 'Campaniel Hotels', null),
        (3673, 'Ibusz Hotels', null),
        (3674, 'Rantasipi Hotels', null),
        (3675, 'Interhotel Cedok', null),
        (3676, 'Отели, мотели, курорты', null),
        (3677, 'Climat De France Hotels', null),
        (3678, 'Cumulus Hotels', null),
        (3679, 'Danubius Hotel', null),
        (3680, 'Отели, мотели, курорты', null),
        (3681, 'Adams Mark Hotels', null),
        (3682, 'Allstar Inns', null),
        (3683, 'Отели, мотели, курорты', null),
        (3684, 'Budget Host Inns', null),
        (3685, 'Budgetel Hotels', null),
        (3686, 'Suisse Chalets', null),
        (3687, 'Clarion Hotels', null),
        (3688, 'Compri Hotels', null),
        (3689, 'Consort Hotels', null),
        (3690, 'Courtyard By Marriott', null),
        (3691, 'Dillion Inns', null),
        (3692, 'Doubletree Hotels', null),
        (3693, 'Drury Inns', null),
        (3694, 'Economy Inns Of America', null),
        (3695, 'Embassy Suites', null),
        (3696, 'Exel Inns', null),
        (3697, 'Farfield Hotels', null),
        (3698, 'Harley Hotels', null),
        (3699, 'Midway Motor Lodge', null),
        (3700, 'Motel 6', null),
        (3701, 'Guest Quarters (formally Pickett Suite Hotels)', null),
        (3702, 'The Registry Hotels', null),
        (3703, 'Residence Inns', null),
        (3704, 'Royce Hotels', null),
        (3705, 'Sandman Inns', null),
        (3706, 'Shilo Inns', null),
        (3707, 'Shoney’s Inns', null),
        (3708, 'Отели, мотели, курорты', null),
        (3709, 'Super8 Motels', null),
        (3710, 'The Ritz Carlton Hotels', null),
        (3711, 'Flag Inns (Ausralia)', null),
        (3712, 'Golden Chain Hotel', null),
        (3713, 'Quality Pacific Hotel', null),
        (3714, 'Four Seasons Hotel (Australia)', null),
        (3715, 'Farifield Inn', null),
        (3716, 'Carlton Hotels', null),
        (3717, 'City Lodge Hotels', null),
        (3718, 'Karos Hotels', null),
        (3719, 'Protea Hotels', null),
        (3720, 'Southern Sun Hotels', null),
        (3721, 'Hilton Conrad', null),
        (3722, 'Wyndham Hotel And Resorts', null),
        (3723, 'Rica Hotels', null),
        (3724, 'Iner Nor Hotels', null),
        (3725, 'Seaines Planation', null),
        (3726, 'Rio Suites', null),
        (3727, 'Broadmoor Hotel', null),
        (3728, 'Bally’s Hotel And Casino', null),
        (3729, 'John Ascuaga’s Nugget', null),
        (3730, 'Mgm Grand Hotel', null),
        (3731, 'Harrah’s Hotels And Casinos', null),
        (3732, 'Opryland Hotel', null),
        (3733, 'Boca Raton Resort', null),
        (3734, 'Harvey/bristol Hotels', null),
        (3735, 'Отели, мотели, курорты', null),
        (3736, 'Colorado Belle/Edgewater Resort', null),
        (3737, 'Riviera Hotel And Casino', null),
        (3738, 'Tropicana Resort And Casino', null),
        (3739, 'Woodside Hotels And Resorts', null),
        (3740, 'Townplace Suites', null),
        (3741, 'Millenium Broadway Hotel', null),
        (3742, 'Club Med', null),
        (3743, 'Biltmore Hotel And Suites', null),
        (3744, 'Carefree Resorts', null),
        (3745, 'St. Regis Hotel', null),
        (3746, 'The Eliot Hotel', null),
        (3748, 'Welesley Inns', null),
        (3749, 'The Beverly Hills Hotel', null),
        (3750, 'Crowne Plaza Hotels', null),
        (3751, 'Homewood Suites', null),
        (3752, 'Peabody Hotels', null),
        (3753, 'Greenbriah Resorts', null),
        (3754, 'Amelia Island Planation', null),
        (3755, 'The Homestead', null),
        (3756, 'South Seas Resorts', null),
        (3757, 'Отели, мотели, курорты', null),
        (3758, 'Отели, мотели, курорты', null),
        (3759, 'Отели, мотели, курорты', null),
        (3760, 'Отели, мотели, курорты', null),
        (3761, 'Отели, мотели, курорты', null),
        (3762, 'Отели, мотели, курорты', null),
        (3763, 'Отели, мотели, курорты', null),
        (3764, 'Отели, мотели, курорты', null),
        (3765, 'Отели, мотели, курорты', null),
        (3766, 'Отели, мотели, курорты', null),
        (3767, 'Отели, мотели, курорты', null),
        (3768, 'Отели, мотели, курорты', null),
        (3769, 'Отели, мотели, курорты', null),
        (3770, 'Отели, мотели, курорты', null),
        (3771, 'Отели, мотели, курорты', null),
        (3772, 'Отели, мотели, курорты', null),
        (3773, 'Отели, мотели, курорты', null),
        (3774, 'Отели, мотели, курорты', null),
        (3775, 'Отели, мотели, курорты', null),
        (3776, 'Отели, мотели, курорты', null),
        (3777, 'Отели, мотели, курорты', null),
        (3778, 'Отели, мотели, курорты', null),
        (3779, 'Отели, мотели, курорты', null),
        (3780, 'Отели, мотели, курорты', null),
        (3781, 'Отели, мотели, курорты', null),
        (3782, 'Отели, мотели, курорты', null),
        (3783, 'Отели, мотели, курорты', null),
        (3784, 'Отели, мотели, курорты', null),
        (3785, 'Отели, мотели, курорты', null),
        (3786, 'Отели, мотели, курорты', null),
        (3787, 'Отели, мотели, курорты', null),
        (3788, 'Отели, мотели, курорты', null),
        (3789, 'Отели, мотели, курорты', null),
        (3790, 'Отели, мотели, курорты', null),
        (3791, 'Отели, мотели, курорты', null),
        (3792, 'Отели, мотели, курорты', null),
        (3793, 'Отели, мотели, курорты', null),
        (3794, 'Отели, мотели, курорты', null),
        (3795, 'Отели, мотели, курорты', null),
        (3796, 'Отели, мотели, курорты', null),
        (3797, 'Отели, мотели, курорты', null),
        (3798, 'Отели, мотели, курорты', null),
        (3799, 'Отели, мотели, курорты', null),
        (3800, 'Homestead Suites', null),
        (3801, 'Wilderness Hotel and Resort', null),
        (3802, 'The Palace Hotel', null),
        (3803, 'The Wigwam Golf Resort and Spa', null),
        (3804, 'The Diplomat Country Club and Spa', null),
        (3805, 'The Atlantic', null),
        (3806, 'Princeville Resort', null),
        (3807, 'Element', null),
        (3808, 'LXR (Luxury Resorts)', null),
        (3809, 'Settle Inn', null),
        (3810, 'La Costa Resort', null),
        (3811, 'Premier Travel Inns', null),
        (3812, 'Hyatt Place', null),
        (3813, 'Hotel Indigo', null),
        (3814, 'The Roosevelt Hotel NY', null),
        (3815, 'Nickelodeon Family Suites by Holiday Inn', null),
        (3816, 'Home2Suites', null),
        (3817, 'Affinia', null),
        (3818, 'Mainstay Suites', null),
        (3819, 'Oxford Suites', null),
        (3820, 'Jumeirah Essex House', null),
        (3821, 'Caribe Royal', null),
        (3822, 'Crossland', null),
        (3823, 'Grand Sierra Resort', null),
        (3824, 'Aria', null),
        (3825, 'Vdara', null),
        (3826, 'Autograph', null),
        (3827, 'Galt House', null),
        (3828, 'Cosmopolitan of Las Vegas', null),
        (3829, 'Country Inn By Radisson', null),
        (3830, 'Park Plaza Hotel', null),
        (3831, 'Waldorf', null),
        (3832, 'Curio Hotels', null),
        (3833, 'Canopy', null),
        (3834, 'Baymont Inn & Suites', null),
        (3835, 'Dolce Hotels and Resorts', null),
        (3836, 'Hawthorn by Wyndham', null),
        (3837, 'Hoshino Resorts', null),
        (3838, 'Kimpton Hotels', null),
        (3882, 'Инкассация чека (обналичивание)', 'MCC код и описание найдены в правилах программы лояльности карты Кукуруза. В документации международных платежных систем данного кода нет.'),
        (3990, 'Экосистема Яндекса', 'Отдельный MCC для точек, относящихся к экосистеме Яндекс.

В наименовании мерчанта содержится MCC категории покупки (например, YANDEX*4121*TAXI). Многие банки определяют категорию покупки в связке с ним.'),
        (3991, 'Экосистема Сбера', 'Отдельный MCC для точек, относящихся к экосистеме Сбера.

В наименовании мерчанта содержится MCC категории покупки (например, SBER*8999*SBERPRIME1). Многие банки определяют категорию покупки в связке с ним.'),
        (3992, 'Экосистемы, для которых НСПК определила специфические MCC', 'Экосистемы, для которых определены специфические MCC 3990-3999, отражающие индивидуальное наименование точек в рамках специальной программы межбанковских вознаграждений.'),
        (3993, 'Экосистемы, для которых НСПК определила специфические MCC', 'Экосистемы, для которых определены специфические MCC 3990-3999, отражающие индивидуальное наименование точек в рамках специальной программы межбанковских вознаграждений.'),
        (7273, 'Знакомства', 'Торговцы, предоставляющие услуги знакомств и эскорта, в том числе через компьютеры, личные видео и сервисы знакомств'),
        (3994, 'Экосистемы, для которых НСПК определила специфические MCC', 'Экосистемы, для которых определены специфические MCC 3990-3999, отражающие индивидуальное наименование точек в рамках специальной программы межбанковских вознаграждений.'),
        (3995, 'Экосистемы, для которых НСПК определила специфические MCC', 'Экосистемы, для которых определены специфические MCC 3990-3999, отражающие индивидуальное наименование точек в рамках специальной программы межбанковских вознаграждений.'),
        (3996, 'Экосистемы, для которых НСПК определила специфические MCC', 'Экосистемы, для которых определены специфические MCC 3990-3999, отражающие индивидуальное наименование точек в рамках специальной программы межбанковских вознаграждений.'),
        (3997, 'Экосистемы, для которых НСПК определила специфические MCC', 'Экосистемы, для которых определены специфические MCC 3990-3999, отражающие индивидуальное наименование точек в рамках специальной программы межбанковских вознаграждений.'),
        (3998, 'Экосистемы, для которых НСПК определила специфические MCC', 'Экосистемы, для которых определены специфические MCC 3990-3999, отражающие индивидуальное наименование точек в рамках специальной программы межбанковских вознаграждений.'),
        (3999, 'Экосистемы, для которых НСПК определила специфические MCC', 'Экосистемы, для которых определены специфические MCC 3990-3999, отражающие индивидуальное наименование точек в рамках специальной программы межбанковских вознаграждений.'),
        (4011, 'Железные дороги – перевозка грузов', 'Железные дороги, занимающиеся транспортировкой грузов.
Для железных дорог, занимающихся перевозкой пассажиров, используется MCC 4112.'),
        (4111, 'Пассажирские перевозки - пригородные и местные пригородные рейсы, включая паромы', 'Услуги местного и пригородного общественного пассажирского транспорта по регулярным маршрутам и с регулярным графиком, включая железнодорожные пригородные перевозки.
Для такси и лимузинов, используется MCC 4121; для автобусов используется MCC 4131.'),
        (4112, 'Пассажирские железнодорожные перевозки', 'Железнодорожные компании, которые в основном предоставляют услуги по перевозке пассажиров на длинные расстояния. Такие точки могут предоставлять или не предоставлять ночлег в поезде в течение длительных поездок.
Для железнодорожных компаний, которые предоставляют транспортировку грузов, используется MCC 4011.'),
        (4119, 'Услуги скорой помощи', 'Экстренные транспортные услуги с обученным персоналом, который может или не может оказать неотложную медицинскую помощь по пути в больницу или медицинскую помощь.'),
        (4121, 'Лимузины и такси', 'Услуги пассажирских автомобильных перевозок, которые не следуют по регулярному графику или установленному маршруту.'),
        (4131, 'Автобусные линии', 'Услуги пассажирского транспорта, которые работают по регулярному графику по заранее определенным маршрутам.
Для операторов чартерных и туристических автобусов используется MCC 4722.'),
        (4214, 'Агентства по автотранспортным перевозкам, местные/ дальные автогрузоперевозки, компании попереезду и хранению, местная доставка', 'Местные и дальние автогрузоперевозки, которые могут также предоставлять или не предоставлять услуги по хранению. Для хранения предметов домашнего обихода и хранения без услуг по перевозкам, используется MCC 4225. Для курьеров почтовых посылок, товаров, и бандеролей, используется MCC 4215.'),
        (4215, 'Услуги курьера – по воздуху и на земле, агентство по отправке грузов', 'Торговые точки, занимающиеся доставкой лично адресованных писем, посылок и бандеролей (исключая американские почтовые услуги – MCC 9402).
Для агентств, занимающихся автогрузоперевозками используется MCC 4214.'),
        (4225, 'Складское хранение общественного пользования –сельскохозяйственных продуктов, охлаждаемые продукты, хранение предметов домашнего обихода', 'Поставщики складских помещений для хранения сельскохозяйственных продуктов, охлаждаемое хранение скоропортящихся продуктов, и хранение предметов домашнего обихода и мебели. Для торговых точек, предоставляющих услуги по хранению товаров в определенной местности, а также услуги по перевозке товаров, используется MCC 4214.'),
        (4304, 'Категория неизвестна', 'Код не найден в документации ни одной из платёжных систем, но находится в списках mcc-кодов для категории Авиабилеты у некоторых банков.'),
        (4411, 'Круизные линии', 'Торговые точки, предлагающие пассажирский транспорт по открытому морю или внутренним водоемам с целью отдыха или ради удовольствия. Такие торговые точки обычно предлагают питание, развлечения, и каюты, включая всё в стоимость проезда, а также проведение стандартных и рекламируемых рейсов. С целью предварительного заказа билетов с каютами на круизных суднах, используется TCC с X. Для транзакций, возникающих в торговых точках, находящихся за пределами корабля, например, в магазине одежды, подарков, или цветочном, используется TCC с H.'),
        (4457, 'Аренда и лизинг суден', 'Торговые точки, предлагающие в основном сдачу в наем и в аренду суден, включая рыболовные судна, плавучие дома без экипажа, парусные лодки, катера, водные мотоциклы и яхты.'),
        (4468, 'Пристани для яхт, их обслуживание и поставка расходных материалов', 'Операторы пристаней для яхт. Предоставляемые услуги могут включать аренду буксира, хранение, мойку, мелкий ремонт лодок и розничную продажу расходных материалов для яхт. В этот MCC входят точки, расположенные в пристани, которые продают топливо для использования в лодках и у которых нет отдельного торгового соглашения с пристанью.

Для заправочных станций, расположенных на пристани и имеющих отдельные торговые соглашения, используется MCC 5541.'),
        (4511, 'Авиалинии, авиакомпании - нигде более не классифицированные', 'Только те авиалинии и авиакомпании, которым не выделены MCC коды'),
        (4582, 'Аэропорты, терминалы аэропортов, лётные поля', 'Торговые точки, которые управляют и обслуживают аэропорты и лётные поля. Такие точки могут предложить мойку и уборку самолетов, обслуживание, ремонт самолетов, хранение самолетов в аэропортах и сдача в аренду ангаров аэропортов. Для торговых точек, находящихся внутри терминала аэропорта, торгующих едой, газетами, подарками или сувенирами, используется соответствующий этому бизнесу MCC, например, MCC 5812, 5994 или 5947.'),
        (4722, 'Туристические агентства и организаторы экскурсий', 'Туристические агентства, которые в основном предоставляют туристическую информацию и услуги бронирования.
Такие точки выступают в качестве агентов от имени путешественников при бронировании и покупке авиабилетов, билетов на наземный или морской транспорт или бронирования, включая полеты на самолете, автобусные туры, морские круизы, прокат автомобилей, железнодорожные перевозки и проживание. Также включает в себя туроператоров, которые организуют и собирают туры для продажи через турагента или непосредственно покупателю. Путешественник также может заказать такие туристические пакеты и экскурсии через консьержа отеля или в кассе.'),
        (4723, 'Пакетные туроператоры - только Германия', 'Точки, классифицированные этим MCC, являются туроператорами в Германии.'),
        (4729, 'Услуги пассажирских перевозок – нигде более не классифицированные', 'MCC код не найден, но данный код есть в справочниках SIC-кодов и входит в списки исключений некоторых российских банков. Судя по всему является аналогом MCC 4789.'),
        (4784, 'Платные дороги и мосты', 'Торговые точки, собирающие плату за проезд по платным дорогам, трассам и мостам.'),
        (4789, 'Услуги пассажирских перевозок – нигде более не классифицированные', 'Точки, предлагающие услуги по перевозке пассажиров, нигде более не классифицированные. Такие услуги включают перевозки на конной тяге, велотакси, канатные дороги, трансфер до аэропорта или фуникулер. Не включает услуги парома, автобусные поездки, круизные линии, пассажирские поезда, такси и лимузины.'),
        (4812, 'Телекоммуникационное оборудование, включая продажу телефонов', 'Торговые точки, которые продают телекоммуникационное оборудование, такое как телефоны, факсы, пейджеры, сотовые телефоны, и другое оборудование, относящееся к телекоммуникациям.
Для продаж телекоммуникационных услуг используется MCC 4814.'),
        (4813, 'Торговые точки телеком клавишного ввода, предлагающие единичные локальные и дальние телефонные звонки, используя центральный номер доступа без разговора с оператором и используя код доступа', 'Провайдеры телекоммуникационных услуг, включая локальные и дальние телефонные звонки, используя клавишный ввод посредством центрального номера доступа.'),
        (5415, 'Категория неизвестна', 'Код не найден в документации ни одной из платёжных систем, но находится в списках mcc-кодов для категории Дом/Строительство у некоторых банков.'),
        (4814, 'Телекоммуникационные услуги', 'Провайдеры телекоммуникационных услуг, таких как местные и междугородные телефонные звонки и услуги факса. Включены точки, которые продают предоплаченные телефонные услуги, такие как телефонные карточки, и точки, выполняющие регулярное (например, ежемесячные) выставление счетов за телефонные звонки.'),
        (4815, 'МастерФон телефонные услуги – Ежемесячное составление телефонных счетов', 'Используется исключительно для ежемесячных телефонных счетов для телефоллых услуг МастерКард МастерФон.'),
        (4816, 'Компьютерные сети, информационные услуги', 'Провайдеры компьютерных сетей, информационные услуги и другие онлайн-сервисы, такие как хранилища файлов, электронные доски объявлений, электронная почта, услуги хостинга веб-сайтов или доступа в Интернет.

Для точек, предлагающих продукты или услуги через интернет, используется MCC, который наилучшим образом описывает предлагаемый продукт или услугу.
Для точек, которые предоставляют аудиотекст (например, психологические горячие линии) или видеотекст (например, сайты для взрослых в чате), используется MCC 5967.
Для точек, оказывающих услуги разработки программ и обработки данных, используется MCC 7372.'),
        (4821, 'Услуги телеграфа', 'Провайдеры телеграфных и других коммуникационных услуг по передаче несловесных сообщений, таких как каблограммы.'),
        (4829, 'Денежные переводы', 'Транзакция, при которой средства доставляются или становятся доступными человеку или счету. Эти транзакции включают транзакции не лицом к лицу, а осуществляемые, например, через Интернет.'),
        (4899, 'Кабельные и другие платные телевизионные услуги', 'Подключение и прямая подача телевизионных программ со взносом или на платной основе.'),
        (4900, 'Жилищно-коммунальные услуги', 'Точки, оказывающие услуги по передаче или распределению электроэнергии, газа, по установке и обслуживанию систем водоснабжения или сбору и утилизации отходов.

Для точек, предоставляющих телекоммуникационные услуги, используется MCC 4814.'),
        (5013, 'Поставщики грузовиков и запчастей', 'Оптовые поставщики аксессуаров для грузовиков, инструментов, оборудования и новых запчастей.
Для работ по ремонту автомобилей используется MCC 7531 или 7538, смотря, что больше подходит.'),
        (5021, 'Офисная и торговая мебель', 'Оптовые производители или распространители офисной мебели (например, столы, стулья, шкафы, перегородки, стеллажи) и торговая мебель (например, столы и стулья ля ресторанов, мебель для общественных парков и строений, церковные скамьи, школьные парты, театральные ложи). Для производителей, делающих или продающих оборудование и мебель для больниц, такие, как кровати, используется MCC 5047.'),
        (5039, 'Строительные материалы – нигде более не классифицированные', 'Предприятия оптовой торговли строительными материалами, такими, как здание из сборных материалов, пиломатериалы и стекло, архитектурная металлообработка, навесы, ограждения, металлические строения, и септик танк. Такие торговые точки могут продавать или не продавать краски и красящее оборудование.
Для предприятий оптовой торговли, продающих краски и красящее оборудование, используется MCC 5198.
Для продавцов и распространителей оборудования, используемого в строительных проектах, используется MCC 5046.'),
        (5044, 'Офисное, фотографическое, фотокопировальное, и микрофильмирующее оборудование', 'Предприятия оптовой торговли офисного и фотографического оборудования, такого, как пленки, кассовые аппараты, копировальные машины, микрофильмирующие машины, камеры хранения и сейфы, пишущие машинки, факсовые машины, арифмометры, машины для приклеивания или печати адресов. Для оптовых торговцев офисной мебелью, используется MCC 5021. Для оптовых торговцев компьютерным оборудованием, используется MCC 5045.'),
        (5261, 'Садовые принадлежности (в том числе для ухода за газонами) в розницу', 'Продажа инвентаря для цветочных питомников, саженцев деревьев и кустарника, растений в горшках, семян, луковиц, мульчи, почвоулучшителей, удобрений, пестицидов, садового инвентаря и  других садовых принадлежностей'),
        (5552, 'Зарядка электромобилей', 'Точки, продающие электроэнергию с целью заправки автомобилей'),
        (7523, 'Паркинги и гаражи', 'Компании предоставляющие услуги временного паркования для автомобилей, с ежедневной или помесячной оплатой. На контрактной основе или за отдельную плату.'),
        (5045, 'Компьютеры, периферийное компьютерное оборудование, программное обеспечение', 'Оптовые распространители компьютерного оборудования, программного обеспечения, и соответствующего оборудования, которое может сопровождать или не сопровождать ремонтные работы. Товары для продажи могут включать компьютерные мониторы, драйвер дисковода, кабели, клавиатуры, принтеры, сканеры, модемы, компьютерные программы, и относящиеся аксессуары и оборудование. Такие торговые точки могут также продавать столы и другую офисную мебель, специально разработанную для использования с компьютерами. Для торговых точек, которые в основном продают компьютерную мебель, используется MCC 5021. Для торговых точек, проводящих главным образом ремонтные работы, используется MCC 7379.'),
        (5046, 'Коммерческое оборудование – нигде более не классифицированное', 'Оптовые поставщики торговых машин и оборудования нигде более не классифицированного. Примеры включают пищевое оборудование и оборудование для тепловой обработки, неоновые вывески, весы, шкафчики, торговые печи и микроволновые печи, приборы для газированной воды, торговые прилавки, манекены, и торговые автоматы. Такие торговые точки могут сдавать или не сдавать в аренду или лизинг оборудование.
Для торговых точек, сдающих в аренду или лизинг оборудование, используется MCC 7394.'),
        (5047, 'Стоматологическое / лабораторное / медицинское / офтальмологическое оборудование и материалы для больниц', 'Представители оптовой торговли лабораторным, хирургическим, ортопедическим оборудованием, а также оборудованием для слежения за больными, и колясками для инвалидов, медицинскими инструментами, промышленными средствами безопасности, больничными койками, и другими сопутствующими товарами для больницы. Также включает поставщиков  стоматологических лабораторных, ортопедических, профессиональных устройств, диагностического оборудования, слуховых аппаратов, аптечек, терапевтического оборудования, рентгеновских машин и запасных частей. Для оптовых торговцев мебелью, такой как стулья, столы, журнальные стенды для медицинских или стоматологических приемных,  используется МСС 5021. Для оптовых поставщиков сопутствующих товаров для уборки больниц, используется МСС 2842.'),
        (5051, 'Центры и офисы работ по металлу', 'Оптовые поставщики полуобработанных металлических изделий, исключая драгоценные металлы. Товары для продажи могут включать стальные трубы и трубопроводы, проволочное сито и скрепляющие детали, гвозди, болванки. алюминиевые стержни, рельсы, металлические или оцинкованные листы, металлические полоски, чугунные катанки, и проволочные канаты или кабели. Такие торговые точки могут сотрудничать с оптовыми магазинами (центры работ по металлу) и с не оптовыми (офисы по продаже металла).
Для оптовых торговцев драгоценными металлами, используется МСС 5094.'),
        (5065, 'Электрические части и оборудование', 'Оптовые продавцы электронных частей и коммуникационного оборудования. Товары для продажи могут включать электрические проволоки, конденсаторы, электрические конденсаторы, диоды, полупроводниковые приборы, системы громкой связи, и электрические вывески.
Для розничных торговцев, продающих электронное оборудование, используется МСС 5732.
Для розничных торговцев, продающих телекоммуникационное оборудование, такое, как телефоны и пейджеры, используется МСС 4812.'),
        (5072, 'Оборудование и сопутствующие материалы для технического обеспечения', 'Предприятия оптовой торговли общим техническим обеспечением и ножами. Товары для продажи могут включать болты, гайки, заклепки, отвертки, зажимы, ручные инструменты. замки, шайбы, кнопки, скобы, гвозди, ручные пилы, пильное полотно, электрические ручные инструменты.
Для торговых точек, продающих техническое обеспечение, используется МСС 5251.'),
        (5074, 'Оборудование для водопровода и отопительной системы', 'Оптовые распространители гидравлического оборудования и сопутствующих товаров для водопроводной и отопительной систем. Товары для продажи могут включать вентили, хомуты, оснастку, конвекторы, котлы, медные товары, панели и оборудование для солнечного отопления, и водонагреватели. Для оптовых продавцов торгового морозильного оборудования и сопутствующих товаров, или электроприборов, таких как газовые и электрические кухонные плиты и торговые печи, используется МСС 5046.'),
        (5085, 'Промышленное оборудование –нигде более не классифицированное', 'Оптовые распространители промышленного оборудования, нигде более не классифицированные. Товары для продажи могут включать абразивные материалы, опоры, коробки, металлическая, стеклянная и керамическая тара, контейнеры, пробки, сальники, резиновые втулки, гидравлические вентили, шланги, поршни, цепные колеса, втулки. бутылки (стеклянные или пластиковые), промышленная подгонка, канатно-веревочные изделия, шпагат и трос, металлические крышки, печатная краска для принтеров, гравировальное оборудование, кожаные ремни, сопутствующие товары для станков, ведра, резиновые товары, сопутствующие товары для печатания на ткани, и металлические бочки.'),
        (5094, 'Драгоценные камни и металлы, часы и ювелирные изделия', 'Оптовые торговцы ювелирных изделий, часов, драгоценных камней и металлов. Товары для продажи могут включать дешевые украшения, часы и детали часов, изделия из серебра, жемчуга, бриллианты и другие драгоценные камни, золото, платину и другие драгоценные металлы, и трофеи. Такие торговые точки могут предлагать или не предлагать работы по ремонту. Для торговых точек, занимающихся ремонтом, используется МСС 7699. Для розничных торговцев предметами, перечисленными здесь, используется МСС 5944.'),
        (5099, 'Товары длительного пользования – нигде более не классифицированные', 'Оптовые распространители товаров длительного пользования, нигде более не классифицированные. Товары для продажи могут включать огнетушители, пожарная сигнализация, газовые зажигалки, записанные кассеты, топливная древесина, лесоматериалы, древесная стружка, неэлектрические вывески, музыкальные инструменты, и багаж.
Для оптовых поставщиков электрических вывесок, используется МСС 5065.'),
        (5111, 'Канцелярия, офисные сопутствующие товары, бумага для печатания и письма', 'Оптовые поставщики канцелярии, офисных принадлежностей, и бумаги для печатания и письма. Товары для продажи могут включать деловые формы, копировальное оборудование, регистрационные карточки и папки-скоросшиватели, ручки, карандаши, конверты, ленты для печатных машинок и принтеров, скоросшиватели, квитанционные книжки и книги продаж, альбомы для фотографий и для наклеивания газетных вырезок.
Для торговцев офисными машинами, такими, как печатные и копировальные машины, и для торговых точек, предлагающих копировальные услуги, используется МСС 5044.
Для оптовых торговцев офисной мебелью, используется МСС 5021.'),
        (5122, 'Лекарства, их распространители, аптеки', 'Оптовые распространители предписанных и запатентованных лекарств, витаминов, гигиенической косметики, антисептиков, перевязочных материалов, фармацевтической продукции, биологических и подобных товаров, и других разнообразных мелких товаров, типичных для продажи в аптеках.
Для оптовых торговцев хирургическими, больничными, или стоматологическими товарами, используется МСС 5047.'),
        (5131, 'Штучные товары, галантерея и другие текстильные товары', 'Оптовые поставки штучных и текстильных товаров. Сюда относятся: марля, мануфактурные товары, изделия из стекловолокна, застежки-молнии, натуральные или синтетические ткани, продаваемые по ярду, комплекты для сборки поясов и пряжек, текстильные соединения, пуговицы, трикотажные и кружевные полотна, ленты, швейные принадлежности, нитки и отделочные ткани.
Для оптовых поставок гардин или других  портьерных тканей используется MCC 5719.'),
        (5137, 'Мужская, женская и детская спецодежда', 'Оптовые поставки рабочей одежды и всех видов мужской, женской и детской спецодежды, включая обувь, дневное белье, плащи, мантии и шапочки для выпускников колледжей, форменную одежду для занятий спортом (для балета, каратэ, футбола и т.д.), а также форму для частных или религиозных учебных заведений.
Для оптовых поставщиков спец-обуви используется MCC 5139. 
Для розничных торговцев, специализирующихся на продаже одежды и аксессуаров используется соответственно розничный MCC.'),
        (5139, 'Спец-обувь', 'Оптовые поставки туфель и ботинок специального назначения, включая спортивную обувь. Для розничных торговцев обувью используется MCC 5661'),
        (5412, 'Гибридные супермаркеты', 'Код не найден в документациях, но включен в список кодов категории "Супермаркеты" некоторых банков'),
        (5169, 'Химикалии и смежные вещества - нигде более не классифицированные', 'Оптовые поставки химикалий и смежных веществ, не подпавших под другую категорию. Обычно используются в промышленности. Сюда относятся: промышленные кислоты, аммиак и спирт, тяжелые, ароматические и другие хим. соединения, хлорин, сжатые и сжиженные газы, детергенты, присадки к топливу и присадки для смазочных масел, полимеры, соли, скипидар, уплотнители, антикоррозионные химические вещества, пековые продукты, сухой лед, красители, клей, желатин и взрывчатые вещества.
Для розничной торговли пиротехническими изделиями используется MCC 5999.
Продавцам хим. веществ для чистки и санитарной обработки присваивается MCC 2842.'),
        (5172, 'Нефть и нефтепродукты', 'Оптовые поставки нефти и нефтепродуктов, таких как: бутан, сырая нефть, мазут(жидкое топливо), бензин, керосин, смазочные масла и жиры и нафта(тяжелый бензин). Сюда также относятся поставщики услуг по заправке самолетов. Для розничной торговли жидким топливом, печным топливом, лесом, углем или пропаном используется MCC 5983.'),
        (5192, 'Книги, периодические издания и газеты', 'Оптовые производители и поставщики книг, периодических изданий, журналов и газет. Для издательств книг, периодики и журналов используется MCC 2741. Для розничной продажи книг используется MCC 5942.'),
        (5193, 'Принадлежности для флористов, питомник и цветы', 'Оптовые дистрибьюторы цветов, материалов для питомников и флористов, свежих и искусственных цветов и горшечных растений.
Для розничных магазинов цветов и растений используется MCC 5992.'),
        (5198, 'Лакокрасочная продукция и сопровождающие товары', 'Оптовые поставки красок, лаков, обоев и сопровождающих товаров. Ассортимент включает краски и красители, эмали, лаки, кисти для красок, баночки для красок, наждачную бумагу, шеллак,  валики, распылители  и т.д.
Для розничной торговли вышеупомянутыми товарами используется MCC 5231.'),
        (5199, 'Товары недлительного пользования - нигде более не классифицированные', 'Оптовые дистрибьюторы товаров недлительного пользования, не классифицированные в других MCC. Ассортимент товаров таких торговцев может включать в себя продукты питания, предметы искусства и ремесла, вешалки для одежды, пенорезину, лед, сырую резину, губки, текстильные мешки, мешковину, холщовые изделия, корзины, подарки и новинки, кожа и режущие материалы, а также другие кожаные изделия, кроме обуви.
Для оптовиков, которые в основном продают кожаную обувь, используется MCC 5139.
Для продавцов, которые продают предметы искусства и ремесла, используется MCC 5970; для продавцов подарков и сувениров используется MCC 5947.'),
        (5200, 'Товары для дома', 'Торговые точки, ориентированные на широкую публику, предлагающие богатый выбор товаров для дома, таких как: обои, краски, лесоматериал, садовые принадлежности, электрические и осветительные приборы, а также декорирующие материалы. Эти торговые точки предлагают как "готовые" товары, например, раковины, шкафчики и двери, так и наборы "Сделай сам".'),
        (5211, 'Лесо- и строительный материал', 'Продажа в розницу лесо- и строительного материала. Сюда относятся также строительные компании, предлагающие свою продукцию подрядчиком, а не широкой публике. К товарам, выставляемым на продажу здесь относятся: лесоматериал, незаконченные изделия из дерева, осветительные материалы, цемент, песок, гравий, строительные или электрические материалы, кирпичи, ограждения, трубы, стекловолокно и прессформы. Для крупных складов или сети магазинов, торгующих товарами для дома со скидкой, рассчитанных на широкую публику, используется MCC 5200. Для подрядчиков используется наиболее подходящее MCC из "Договорных услуг"'),
        (5231, 'Розничная продажа стекла, красок и обоев', 'Продажа в розницу стекла, красок и малярных принадлежностей, обоев и сопутствующих товаров. Для оптовых поставщиков малярных принадлежностей используется MCC 5039.'),
        (5251, 'Скобяные товары в розницу', 'Торговые точки, которые продают скобяные товары в полном ассортименте для широкой публики. Сюда относятся такие товары, как: мелкие электрические приборы, провода, гайки, болты, гвозди, шурупы, молотки, отвертки и другие мелкие инструменты, кольцевые прокладки, ключи, лампочки, скобы и сантехническое оборудование. Для крупных складов или сети магазинов, торгующих товарами для дома со скидкой, рассчитанных на широкую публику, используется MCC 5200.'),
        (5262, 'Маркетплейсы', 'Точки, классифицируемые этим MCC, являются онлайн-площадками, которые объединяют держателей карт и розничных продавцов, продающих ряд товаров или услуг, которые могут быть классифицированы разными MCC, на одном веб-сайте или в мобильном приложении под единым брендом, который используется для идентификации покупателями. Торговые площадки с одной линией товаров или услуг должны использовать MCC, который наиболее точно описывает эту сферу деятельности.

MCC пока объявлен только в документации Visa.'),
        (5271, 'Продажа жилых фургонов', 'Торговые точки, занимающиеся продажей новых и б/у жилых фургонов, зап. частей, комплектующих и оборудования к ним.'),
        (5292, 'Категория неизвестна', 'Код не найден в документации ни одной из платёжных систем, но находится в списках mcc-кодов для категории "Аптеки" у некоторых банков.'),
        (5295, 'Категория неизвестна', 'Код не найден в документации ни одной из платёжных систем, но находится в списках mcc-кодов для категории "Аптеки" у некоторых банков.'),
        (5297, 'Retail Internet Volume', 'Код не найден в документациях, но включен в список кодов категории "Супермаркеты" некоторых банков'),
        (5298, 'Internet Shopping Grocery Store', 'Код не найден в документациях, но включен в список кодов категории "Супермаркеты" некоторых банков'),
        (5300, 'Оптовики', 'Склады, специализирующиеся на продаже товаров массового потребления, предлагающие  широкий ассортимент товаров оптом по низким ценам. Такие торговые точки могут иметь или не иметь определенные требования к количеству участников "оптового клуба". Изделия для продажи включают бытовые принадлежности и электроприборы, офисное оборудование, сушеные бакалейные и скоропортящиеся товары, мебель для дома и офиса, электротовары , автозапчасти и аксессуары.'),
        (5309, 'Беспошлинные магазины Duty Free', 'Магазины, торгующие различными сувенирами и импортными товарами, освобожденными от таможенных пошлин, такими как: драгоценности, косметика, табачные и спиртные изделия. Обычно такие торговые точки располагаются в здании аэропорта и отелях.'),
        (5310, 'Магазины, торгующие по сниженным ценам', 'Продажа разнообразных товаров по сниженным ценам. Сюда относится одежда, посуда, бытовое оборудование, принадлежности для личной гигиены и электроприборы. Такие торговые точки обычно находятся у входа в магазин, они рекламируют и продают товары по низким ценам .'),
        (5311, 'Универмаги', 'Крупные торговые точки, имеющие широкий ассортимент товаров в различных секциях с отдельными кассами. Здесь продается одежда, бытовое оборудование, мебель, электроприборы, косметика, посуда и основные бытовые принадлежности'),
        (5331, 'Универсальные магазины', 'Торговцы, предлагающие разнообразный, но ограниченный выбор товаров в низком или популярном ценовом диапазоне. Такие магазины, как правило, не узкоспециализированные, не предлагают проприетарную плату или кредитные карты и не доставляют товар. Универсальные магазины предлагают товары, похожие на проданные дисконтными магазинами, но работают в гораздо меньших масштабах.'),
        (5399, 'Различные товары общего назначения', 'Мелкие торговые точки и магазины среднего звена с широким ассортиментом товаров в различных секциях с отдельными кассами. Здесь торгуют одеждой, текстильными  и скобяными товарами, посудой, бытовыми принадлежностями, электроприборами, мебелью и косметикой. Такие торговые точки предоставляют скидки или отпускают товары в кредит, осуществляют доставку покупок.и продажу со складов. Магазины различных товаров общего назначения предлагают такой же ассортимент товаров, как и универмаги, но в гораздо меньшем обьеме.'),
        (5411, 'Бакалейные магазины, супермаркеты', 'Торговые точки, которые продают полную линейку продуктов питания для домашнего потребления. Пищевые продукты для продажи включают бакалейные товары, мясо, продукты, молочные продукты и консервированные, замороженные, предварительно упакованные и сухие продукты. Также продукты для продажи могут включать ограниченный выбор посуды, чистящих и полирующих изделий, средств личной гигиены, косметики, поздравительных открыток, книг, журналов, предметов домашнего обихода и сухих товаров. Эти точки также могут управлять специализированными отделами, такими как лавка деликатесов, мясная лавка, аптека или цветочный отдел.
Для магазинов, которые продают ограниченный выбор продуктов или предметов специальности, используется MCC 5499.'),
        (5422, 'Продажа свежего и мороженого мяса', 'Продажа свежего, замороженного или консервированного мяса и рыбы, моллюсков и других морепродуктов. Сюда относятся также торговые точки, осуществляющие массовые розничные продажи мяса для хранения в замороженном виде. Такие мясные магазины могут продавать мясо либо собственного скота, либо закупать мясо через другие фирмы.
Для магазинов домашней птицы используется MCC 5499.'),
        (5441, 'Кондитерские', 'Продажа конфет, шоколада, орешков, сухофруктов, попкорна и др.'),
        (5451, 'Продажа молочных продуктов в розницу', 'Торговые точки, непосредственно продающие расфасованные молочные продукты. Сюда относятся: масло, сыр, расфасованное мороженое и молоко.'),
        (5462, 'Булочные', 'Торговые точки, продающие хлебобулочные изделия, а также изготавливающие продукцию на заказ. Сюда относятся: рогалики, хлеб, пирожные, пончики, изделия из слоеного теста, булочки и свадебные торты.'),
        (5499, 'Различные продовольственные магазины - нигде более не классифицированные', 'Торговые точки, продающие продукты, не классифицированные в других категориях. Сюда относятся: специализированные продовольственные рынки, магазины диетических продуктов, деликатесов, домашней птицы, кофейни, овощные и фруктовые рынки, а также магазины мороженого, йогуртов и полуфабрикатов и небольшие магазины у дома.
Для магазинов, которые также продают автомобильный бензин, используется MCC 5541.
Для точек, которые в основном продают мясо и морепродукты, используется MCC 5422.'),
        (5511, 'Легковой и грузовой транспорт – продажа, сервис, ремонт, запчасти и лизинг.', 'Магазины, продающие новые и подержанные автомобили, грузовики, пикапы и микроавтобусы. Эти торговые точки могут также производить ремонтные работы и предоставлять запчасти и аксессуары в ассортименте.
Для точек, специализирующихся на ремонтных работах, используется МСС 7538.'),
        (5521, 'Продажа легковых и грузовых автомобилей (только подержанных)', 'Магазины, торгующие только подержанными автомобилями и грузовиками,  не осуществляющие продажу новых автотранспортных средств.Сюда относятся: подержанные пикапы, микроавтобусы, антикварные и старинные автомобили.'),
        (5531, 'Автомагазины и товары для дома', 'Торговые точки, продающие различные товары для дома, а также для ремонта и усовершенствования автомобилей. Сюда относятся: новые автошины, аккумуляторы и другие автозапчасти и аксессуары, а также бытовые принадлежности, оборудование и техника.
Для торговых точек, реализующих товары и принадлежности для дома со склада, используется МСС 5200.
Для магазинов, продающих автозапчасти, оборудование и сопутствующие аксессуары, используется МСС 5533.'),
        (5532, 'Автошины', 'Торговые точки, продающие шины для автомобилей и грузовиков, а также сопутствующие запасные части. Эти магазины производят также замену шин и ремонтное обслуживание.
Для торговых точек, специализирующихся на восстановлении протекторов и починке шин, используется МСС 7534.'),
        (5533, 'Автозапчасти и аксессуары', 'Торговые точки, продающие автозапчасти, оборудование и аксессуары. Сюда относятся: масла и масляные фильтры, запчасти, глушители, чистящие и полирующие средства, свечи зажигания, освежители, стеклоочистители и краски. В отдельных случаях здесь также могут продаваться автомагнитолы и усилители.
Для магазинов, специализирующихся на продаже автомагнитол и усилителей, используется МСС 5732.'),
        (5541, 'Заправочные станции (с вспомогательными услугами или без)', 'Торговые точки, которые продают топливо для потребительского использования и могут или не могут также иметь на территории магазин, автомойку или авторемонтную мастерскую. Этот MCC включает станции техобслуживания, расположенных в гавани, у которых есть отдельное торговое соглашение от торгового терминала.
Для транзакций, совершаемых на автоматических заправочных станциях, используется MCC 5542.'),
        (5542, 'Автоматические заправочные станции', 'Торговые точки, которые продают автомобильный бензин, используя, как правило, автоматические топливораздаточные колонки, позволяя держателям карт оплачивать топливо на колонке.'),
        (5551, 'Продажа лодок', 'Торговые точки, продающие новые и подержанные плавсредства, водные принадлежности и подвесные моторы. Сюда относятся: моторные лодки, катера, парусники, рыболовные суда, специализированные лодки для занятия водными лыжами.'),
        (5561, 'Дома- автоприцепы, жилые неразборные и грузовые прицепы', 'Торговые точки, продающие новые и подержанные дома-автоприцепы, прицепы для отдыха и грузовые прицепы. Сюда относятся: жилые неразборные прицепы, прицепы с откидным верхом,    сопутствующие запчасти и аксессуары.'),
        (5571, 'Продажа мотоциклов', 'Торговые точки, продающие новые и подержанные мотоциклы, скутеры, мопеды, сопутствующие запчасти, оборудование и аксессуары. В отдельных случаях здесь также могут продаваться: шлемы, одежда для мотоциклистов – куртки, брюки, головные уборы и перчатки.'),
        (5592, 'Продажа домов на колесах', 'Торговые точки, продающие новые и подержанные дома на колесах и сопутствующие запчасти и аксессуары.
Для домов-автоприцепов и жилых неразборных прицепов используется МСС 5561.'),
        (5598, 'Продажа снегоходов', 'Используется исключительно для торговых точек, продающих новые и подержанные снегоходы, сопутствующие запчасти и аксессуары.'),
        (5599, 'Продажа различного рода автомобилей, авиа- и с/х оборудования - нигде более не классифицированные', 'Торговые точки, занимающиеся продажей новых и б/у автомобилей, авиасредств и с/х техники, запчастей к ним, оборудования и  сопутствующих товаров. На продажу здесь могут выставляться вездеходы, багги с широкопрофильными шинами, миниавтомобили для гольфа, легкие открытые коляски, снегоходы, тракторы, комбайны и уборочные машины'),
        (5611, 'Мужская одежда и аксессуары, включая одежду для мальчиков', 'Продажа готовой мужской одежды и аксессуаров, включая одежду для мальчиков. Сюда также относятся торговые точки, занимающиеся продажей галстуков, и магазины головных уборов'),
        (5621, 'Готовая женская одежда', 'Продажа разнообразной готовой одежды для женщин, например, платьев, брюк, костюмов и пальто. Сюда относятся и магазины, специализирующиеся на продаже свадебных платьев или одежды для беременных.'),
        (5631, 'Аксессуары для женщин', 'Продажа различных женских аксессуаров, включая сумочки, головные уборы, дешевые украшения, шарфы, пояса, заколки для волос и береты, белье, колготки и чулки.'),
        (5641, 'Детская одежда, включая одежду для самых маленьких', 'Продажа детской одежды, принадлежностей и аксессуаров.'),
        (5651, 'Одежда для всей семьи', 'Торговые точки, занимающиеся продажей мужской, женской и детской одежды, принадлежностей и аксессуаров, не специализируясь на конкретной половой или возрастной категории. Сюда же относятся магазины джинсовой или кожаной одежды и магазины одежды для мужчин  и женщин.'),
        (5655, 'Спортивная одежда, одежда для верховой езды и езды на мотоцикле', 'Продажа одежды для людей, ведущих активный образ жизни, одежды для занятий спортом, легкой атлетикой, верховой ездой или для езды на мотоцикле. Такие торговые точки могут специализироваться на одежде для верховой езды, одежде ковбойского стиля, одежде для езды на мотоцикле или мотокросса и могут также выставлять на продажу спортивную обувь и ковбойские сапоги.'),
        (5661, 'Обувные магазины', 'Продажа мужской, женской и детской обуви, включая спортивную обувь. В таких магазинах часто также продаются в ограниченном ассортименте сумки, носки, декоративные элементы для обуви, крем для обуви, перчатки и чулочные изделия.'),
        (5681, 'Изготовление и продажа меховых изделий', 'Продажа  в розницу разнообразных изделий из натурального меха, включая шубы, куртки, головные уборы и перчатки.'),
        (5691, 'Магазины мужской и женской одежды', 'Продажа мужской и женской одежды и аксессуаров. Эти торговые точки не занимаются продажей детской одежды.'),
        (5697, 'Услуги по переделке, починке и пошиву одежды', 'Выполнение и продажа одежды по индивидуальным заказам, переделка, починка.  Сюда относятся также торговые точки, которые специализируются на восстановлении старинной одежды и создании оригинальных костюмов.'),
        (5698, 'Парики и накладки из искусственных волос', 'Продажа накладок постоянного или временного ношения, париков, фальшивых локонов и искусственных прядей для мужчин и женщин, сюда относятся также торговые точки, предоставляющие услуги по завивке.
Для торговых точек, которые предоставляют услуги по наращиванию волос, требующие хирургического вмешательства, используется MCC 8099.'),
        (5699, 'Различные магазины одежды и аксессуаров', 'Продажа специализированной одежды (кроме изделий из меха) и аксессуаров, не классифицированных ранее. Сюда, например, относятся торговые точки, специализирующиеся на продаже теннисок, форменной одежды, купальных костюмов. Для торговых точек, специализирующихся на меховых изделиях, используется MCC 5681.'),
        (5712, 'Оборудование, мебель и бытовые принадлежности (кроме электрооборудования)', 'Торговые точки, занимающиеся продажей бытовых принадлежностей, предлагающие широкий выбор мебели для дома и аксессуаров. Сюда относятся постельные принадлежности и матрацы, мебель для столовой и гостиной, обстановка для веранды или патио, для детской комнаты, а также светильники, коврики и занавески. Такие торговые точки могут заниматься продажей электроприборов в ограниченном ассортименте (например, телевизоров, стерео- и видеомагнитофонов.).
Для торговых точек, специализирующихся только на продаже электрооборудования, используется MCC 5732.'),
        (5713, 'Покрытия для пола', 'Продажа различных покрытий для пола, таких как ковры и ковровые покрытия, плитка для пола, линолеум, камень, паркет или кирпичи. Такие торговые точки могут также предоставлять услуге по установке.
Для торговых точек, занимающихся исключительно установкой, используется MCC 1799.'),
        (5714, 'Ткани, обивочный материал, гардины и портьеры, жалюзи', 'Продажа тканей, занавесей, жалюзи, штор и обивочного материала.
Для торговых точек, преимущественно занимающихся обивкой или починкой мебели, используется MCC 7641.'),
        (5715, 'Оптовые продавцы алкоголя', 'Найдено в сомнительном источнике, но этот код присутствует в категории "Супермаркеты" некоторых банков.'),
        (5718, 'Продажа каминов, экранов для каминов и аксессуаров', 'Торговые точки, продающие камины, деревянные печи, отдельные части каминов и аксессуары, такие как инструменты и экраны для каминов.
Для подрядчиков, выполняющих каменную кладку и установку каминов используется МСС 1740.'),
        (5719, 'Различные специализированные магазины бытовых принадлежностей', 'Торговые точки, продающие различные бытовые принадлежности. Сюда относятся: посуда, кухонные ножи, постельное белье и принадлежности, лампы и абажуры, зеркала, картины, гончарные и керамические изделия.
Для торговцев изделиями из стекла и хрусталя используется МСС 5950.'),
        (5722, 'Бытовое оборудование', 'Торговые точки, продающие бытовое оборудование. Сюда относятся: газовые или электрические печи и плиты, духовки, холодильники, посудомоечные машины, водонагреватели, стиральные машины, сушилки, корзины для мусора, швейные машинки, автономные кондиционеры и пылесосы. Такие торговцы могут выполнять также ремонтные работы.
Для торговцев, занимающихся преимущественно ремонтом, используются МСС 7623.'),
        (5732, 'Продажа электронного оборудования', 'Торговые точки, продающие широкий спектр электронного оборудования, а также отдельных частей и аксессуаров для ремонта, сборки или монтажа электронного оборудования. Могут также выполнять ремонтные работы. Товары для продажи включают в себя: телевизоры, радио, кассетные видеомагнитофоны, видеокамеры и стереосистемы.
Для торговых точек, занимающихся исключительно ремонтом электроники, используется МСС 7622.'),
        (5733, 'Продажа музыкальных инструментов, фортепиано, нот.', 'Торговые точки, продающие музыкальные инструменты, ноты, электромузыкальные клавишные инструменты, самоучители, пианино, гитары, сопутствующие товары и оборудование; могут также предлагать музыкальные консультации. Для магазинов, осуществляющих в основном музыкальные консультации, используется МСС 8299.'),
        (5734, 'Продажа компьютерного программного обеспечения', 'Торговые точки, продающие компьютерные программы для делового и личного пользования и могут продавать или передавать в лизинг ограниченный ассортимент компьютерного оборудования и других сопутствующих товаров.
Для преимущественной продажи и лизинга компьютерной техники и электроники используется МСС 5732.
Для торговых точек, предоставляющих услуги по обработке данных и консалтингу, используется МСС 7372.'),
        (5735, 'Магазины звукозаписи', 'Торговые точки, продающие пластинки, компакт диски (СD), кассеты, музыкальные и видео лазерные диски, чистые аудио- и видеокассеты, а также занимающиеся прокатом видеокассет.
Для торговых точек, занимающихся преимущественно прокатом видеокассет, используется МСС 7841.'),
        (5811, 'Поставщики провизии', 'Торговые точки, занимающиеся приготовлением и доставкой (обычно по конкретному адресу) еды и напитков для немедленного потребления. Поставщики таких услуг также могут или не могут предоставлять услуги по очистке, столы, обслуживающее оборудование, украшения и персонал для обслуживания и уборки на месте.
Для ресторанов, которые в основном подают питание для потребления в помещениях, а также предоставляют услуги общественного питания, используется MCC 5812.'),
        (5812, 'Места общественного питания, рестораны', 'Торговые точки, занимающиеся приготовлением еды и напитков для немедленного потребления, обычно на заказ. Могут оказывать услуги по обслуживанию столиков официантами. Места общественного питания включают в себя: кафе, кафетерии, грили, кофейни, закусочные, охлаждаемые прилавки для продажи мороженого и напитков, пиццерии, столовые, магазины деликатесов, бистро.

Для точек, продающих еду быстрого приготовления, используется МСС 5814.  Для точек, торгующих алкогольными напитками на заказ, используется МСС 5813.'),
        (5813, 'Бары, коктейль-бары, дискотеки, ночные клубы и таверны – места продажи алкогольных напитков', 'Торговые точки, продающие алкогольные напитки, такие как вино, пиво, эль, смешанные напитки и другие ликеры и напитки для потребления на заказ. Места продажи алкогольных напитков включают в себя: бары, пивные, пабы, коктейль-бары, дискотеки, ночные клубы, таверны и винные бары.'),
        (5814, 'Фастфуд', 'Торговые точки, продающие готовую еду и напитки для немедленного потребления, как на заказ, так и упакованную на вынос. Такие рестораны могут использовать мак-драйв для приема и выдачи заказов и обычно не предоставляют услуги по обслуживанию столиков официантами и не берут чаевые. Эти торговые точки обычно не продают алкогольные напитки.'),
        (5815, 'Цифровые товары - аудиовизуальные медиа, включая книги, фильмы и музыку', 'Точки, которые продают аудиовизуальные произведения держателю карты в цифровом формате. Такие работы предоставляются посредством электронной передачи (например, скачивание или потоковая передача) и включают в себя, например, аудиокниги, музыкальные файлы, рингтоны, фильмы, видеозаписи, живые или записанные события, цифровые периодические издания или журналы, цифровые фотографии, цифровые презентации, а также новости и развлекательные программы.'),
        (5816, 'Цифровые товары – игры', 'Торговые точки, которые разрабатывают видео- или электронные игры для игр на смартфонах, оснащены телефонами, персональными компьютерами, планшетами, консолями или другими устройствами с сетевыми возможностями. Такие игры могут предоставлять платформы для совершения покупок электронных или виртуальных предметов in-app для использования во время игры, включая, помимо прочего, игровые элементы, жетоны, очки или другие формы игровой ценности.'),
        (5817, 'Цифровые товары - приложения (кроме игр)', 'Точки, которые продают предварительно написанные программные приложения, доступные держателю карты через удаленный доступ (например, сервер) или скачиваемые.
Для игр используется MCC 5816.'),
        (5818, 'Цифровые товары - мультикатегория', 'Точки, которые продают по крайней мере два из следующих указанных цифровых продуктов:
- MCC 5815 (аудиовизуальные медиа, включая книги, фильмы и музыку)
- MCC 5816 (игры)
- MCC 5817 (приложения, исключая игры)
Цифровой товар передается электронным путем покупателю и получен с помощью иных средств, чем материальные носители.'),
        (5912, 'Аптеки', 'Точки, которые продают лекарственные средства, отпускаемые по рецепту и запатентованному препарату, и лекарства без рецепта (внебиржевые). В аптеках также могут продаваться сопутствующие товары и изделия из него, такие как косметика, туалетные принадлежности, табак, грелки, задние опоры, товары для новинок, открытки и некоторый запас продуктов питания. Продавцы напитков и закусочные в аптеках также должны использовать этот MCC.'),
        (5921, 'Магазины с продажей спиртных напитков навынос', 'Точки, которые продают упакованные алкогольные напитки, такие как эль, пиво, вино и ликер для потребления вне помещений. Такие точки могут или не могут также продавать ограниченное количество закусок, газет и журналов, а также туалетные принадлежности и лекарства без рецепта.
Для точек, которые продают ликер для потребления в помещениях, используйте MCC 5813.'),
        (5931, 'Секонд-хенды, магазины б/у товаров, комиссионки', 'Точки, которые продают использованные или подержанные товары. Товары для продажи могут включать аксессуары, обувь и одежду, мебель, книги, велосипеды, музыкальные инструменты, швейные машины, электронное оборудование, приборы и другие предметы домашнего обихода. Как правило, предметы для продажи были пожертвованы или отправлены на консигнацию. Такие точки могут или не могут также продавать антиквариат.
Для точек, которые в основном продают антиквариат, используется MCC 5932.
Для ломбардов используется MCC 5933.'),
        (5932, 'Антикварные магазины – продажа, ремонт и услуги реставрации', 'Точки, которые продают антиквариат и могут или не могут также выполнять некоторые ремонтные или реставрационные работы. Товары для продажи могут включать мебель, ювелирные изделия, камеры и фотооборудование, инструменты, произведения искусства, книги, бытовую технику и другие предметы домашнего обихода.
Для точек, продающих старинные автомобили, используется MCC 5521.
Для точек, которые в основном ремонтируют или восстанавливают антикварную мебель, используется MCC 7641.'),
        (5933, 'Ломбарды', 'Точки, которые одалживают деньги в обмен на личную собственность, которая остается в точке в качестве обеспечения. Ломбарды могут после невозврата займа продавать собственность широкой публике. Собственность, оставленная в качестве обеспечения, может включать такие предметы, как ювелирные изделия, часы, музыкальные инструменты, велосипеды, мебель, монеты, камеры и фотооборудование.'),
        (5935, 'Уничтожение и сбор остатков', 'Точки, которые предоставляют услуги по уничтожению и сбору остатков транспортных средств, оборудования и других предметов. Такие точки могут или не могут предоставлять услуги по доставке и буксировке.
Для точек, которые в основном предоставляют услуги буксировки, используется MCC 7549.'),
        (5937, 'Магазины репродукций и антиквариата', 'Точки, которые продают антикварные репродукции или факсимиле - точные копии. Товары для продажи могут включать одежду, мебель, ковры, картины и произведения искусства, зеркала, изделия из стекла, ювелирные изделия и другие предметы домашнего обихода и предметы личной гигиены. Такие точки могут также предоставлять услуги по воспроизведению и восстановлению старинных фонографов.
Для точек, которые продают добросовестный ("bona fide") антиквариат, используется MCC 5932.'),
        (5940, 'Веломагазины – продажа и обслуживание', 'Точки, которые продают велосипеды, детали и аксессуары. Веломагазины могут также продавать ограниченное разнообразие спортивной одежды, а также могут или не могут также проводить ремонтные работы или давать велосипеды в аренду.
Для торговцев, которые в основном ремонтируют велосипеды, используйте MCC 7699. Для торговцев, которые в основном дают велосипеды в аренду, используйте MCC 7999.'),
        (5941, 'Спорттовары', 'Торговые точки, торгующие спортивными товарами, спортивным инвентарем и сопутствующими частями и аксессуарами. В продажу могут входить скейтборды, водные лыжи, роликовые коньки, доски для серфинга, оборудование для гольфа, снежные лыжи, виндсерфинг, дайвинг и подводное снаряжение, приманка и снасти, альпинизм, кемпинг, пешеходное и альпинистское снаряжение, оборудование для тренировок, бильярдные столы, теннис оборудование, оборудование и принадлежности для боулинга, оборудование для охоты и снаряжения, оборудование для игровых площадок. Магазины спортивных товаров могут или не могут также проводить ремонтные работы или арендовать спортивное снаряжение.
Для торговцев, которые в основном проводят ремонтные работы, используется MCC 7699.
Для торговцев, которые в основном дают спортивное оборудование в аренду, используется MCC 7999.'),
        (5942, 'Книжные магазины', 'Торговые точки, которые продают новые или б/у книги, журналы, учебники, карты и атласы, аудиокниги и календари. Этот MCC также включает в себя религиозные книжные магазины.'),
        (5977, 'Магазины косметики', 'Розничные торговые точки, которые продают натуральную или синтетическую косметику, в том числе театральный грим, медицинскую и повседневную косметику.'),
        (7338, 'Быстрое копирование, репродуцирование и услуги по созданию чертежей', 'Точки, которые воспроизводят текст, рисунки, планы, карты и подобные материалы путем создания чертежей, фотокопирования или использования других методов воспроизведения. Такие точки также могут предоставлять услуги сортировки и сшивания.'),
        (5943, 'Магазины офисных, школьных принадлежностей, канцтоваров', 'Торговцы, которые продают различные офисные и школьные принадлежности и бумажные товары. Товары для продажи могут включать в себя ручки, карандаши, календари, настольные органайзеры, степлеры, папки с файлами, бумагу для бумаг, портфели, почтовые ящики, маркеры, компьютерные дискеты, чернильные картриджи для компьютерных принтеров, ограниченный набор программного обеспечения для компьютеров и малое офисное оборудование, такое как офисные шкафы, стулья, мусорные корзины и настольные лампы. Такие торговые точки дополнительно продают большое или дорогостоящее офисное оборудование, такое как компьютеры или настольные компьютеры.
Для торговых точек, которые в основном продают компьютерное оборудование, используется MCC 5732.
Для торговых точек, которые в основном продают компьютерное программное обеспечение, используется MCC 5734.'),
        (5944, 'Часы, ювелирные и серебряные изделия', 'Торговые точки, которые продают часы и ручные часы; драгоценные металлы; мелкие ювелирные изделия, такие как алмазы или другие драгоценные камни, смонтированные в драгоценных металлах; и стерлингового серебра и покрытых столовых приборов и аксессуаров, таких как тарелки, сервировочные чаши, триплеты и кувшины. Такие торговые точки могут также проводить ремонтные работы.
Для торговых точек, которые в основном ремонтируют ювелирные изделия, часы или часы, используется MCC 7699.
Для торговых точек, которые в основном продают фарфор или кристалл, используется MCC 5950.'),
        (5945, 'Игрушки, игры и хобби-товары', 'Торговые точки, которые продают игрушки и игры, и могут продавать ограниченный выбор самодельных ремесел или наборов для хобби.
Для торговых точек, которые в основном продают ресурсы, материалы и оборудование, используемые для сборки ремесел, используется MCC 5970.'),
        (5946, 'Магазины фотооборудования и фотоприборов', 'Торговые точки, которые специализируются на продаже фотоаппаратов, плёнки, видео и другого фотооборудования и расходных материалов, включая химикаты для обработки и бумагу. Такие торговые точки могут или не могут также предоставлять услуги по обработке фильмов.
Для торговых точек, которые продают камеры и видеооборудование в дополнение к множеству других электронных устройств, таких как видеомагнитофоны, телевизоры и стереосистемы, используется MCC 5732.
Для торговых точек, которые в первую очередь предоставляют услуги по обработке и печати фильмов, используется MCC 7395.'),
        (5947, 'Магазины открыток, подарков, новинок и сувениров', 'Торговые точки, которые продают подарки и новинки, поздравительные открытки, воздушные шары, сувениры, праздничные украшения, канцелярские принадлежности, оберточные бумаги и банты, фотоальбомы и ограниченный набор канцелярских принадлежностей, таких как ручки, ноутбуки и календари.
Для торговых точек, которые в основном продают офисные и школьные принадлежности, используется MCC 5943.'),
        (5948, 'Магазины кожаных изделий, дорожных принадлежностей', 'Торговцы, которые продают дорожные сумки, сундуки, портфели, кожаные рюкзаки, кошельки, бумажники, перчатки и другие изделия из кожи, и могут продавать ограниченный выбор кожаной одежды.'),
        (5949, 'Магазины ткани, ниток, рукоделия, шитья', 'Торговые точки, которые продают ткани, выкройки, пряжу и другие материалы для шитья и рукоделия такие как нитки, пуговицы, заклепки, подкладочная ткань, шнурки, отделки, ножницы, кружева и застежки-молнии. Такие торговые точки могут также предлагать консультации по шитью.
Для торговых точек, которые прежде всего предлагают инструкции по шитью и вязанию, используется МСС 8299.'),
        (5950, 'Магазины хрусталя и изделий из стекла', 'Торговые точки, которые продают посуду, фарфор и хрусталь для сервировки стола (например, бокалы с вином и шампанским, сервировочные чаши и тарелки), а также подарочные изделия (например, статуэтки, подставки для книг, цветочные вазы, шкатулки для драгоценностей и подсвечники).'),
        (5978, 'Магазины печатающих устройств – аренда, продажа, услуги', 'Розничные торговые точки, которые продают, сдают в аренду, разные печатающие устройства. Такие торговые точки могут также продавать ограниченный выбор офисных принадлежностей, канцтоваров, и могут проводить или не проводить починные работы.
Для точек, которые в основном занимаются починкой подобных товаров, используется МСС 7699.'),
        (5960, 'Прямой маркетинг – страховые услуги', 'Торговые точки, которые продают страховые услуги через прямую почтовую рассылку, накладные выписки по счетам, а также журналы или телевизионные объявления, все из которых включают либо бесплатный номер телефона, либо адрес или веб-сайт, на который потенциальные клиенты могут ответить. Предлагаемые страховые услуги могут включать все формы страхования жизни, страхование от страхового возмещения (дополнительное покрытие, обычно оплачиваемое непосредственно потребителю), страхование от случайной смерти и разделения, страхование по кредитным картам, в которых непогашенный остаток застрахован от смерти, инвалидности или страхования по безработице, и медицинская страховка для путешественников. Продажи могут быть нацелены на специальные группы, такие как ветераны, пенсионеры, школьные учителя или члены определенных организаций. Биллинг страховых премий обычно происходит в периодических (ежемесячных, ежеквартальных или годовых) взносах и продолжается до тех пор, пока не будет отменен держателем карты или страховой компанией. Еще одна общая особенность - бесплатный пробный период в 60 или 90 дней с выставлением счета первой партии после окончания пробного периода.
Для личных услуг по страхованию и гарантированию используется MCC 6300.'),
        (5961, 'Заказы по почте, включая заказы по каталогу', 'MCC отсутствует в справочниках, найдено в справочниках SIC кодов'),
        (5962, 'Прямой маркетинг – услуги, относящиеся к туризму', 'Торговые точки, которые продают услуги по организации поездок с помощью исходящего телемаркетинга, в которых продавец начинает контактировать с потребителем посредством прямой почты, рекламы или других методов прямого маркетинга, требующих ответа потребителей, в попытке произвести продажу. Также включены дисконтные туристические клубы и подписные услуги или информационные бюллетени, из которых подписчики выбирают расфасованные поездки; для них часто требуется ежегодный членский взнос, который может быть выставлен счету держателя карты каждый год, пока он не будет отменен владельцем карты или торговцем.
Для турагентов и туроператоров используется MCC 4722.'),
        (5963, 'Продажа "от двери до двери"', 'Продавцы, которые продают товары "от двери до двери" или из грузовых автомобилей или вагонов или других временных мест. Товары, продаваемые от двери до двери, могут включать косметику, предметы домашнего обихода, хлебобулочные изделия, молочные продукты и подписки на журналы.'),
        (5964, 'Прямой маркетинг – торговля по каталогам', 'Торговые точки, которые инициируют прямой контакт с потребителями и часто описываются как дома заказов по почте или как индустрия "товары почтой". Такие торговые точки предлагают свои товары через каталоги и принимают заказы на товары исключительно по электронной почте, телефону, факсу, электронной торговле или другим методам, не предусматривающих личный контакт. Бумажный каталог - это многостраничный документ, который отправляется по почте, факсом и т.п. непосредственно потребителю. Электронный каталог отображает товар через кабельное телевидение или видеотекст на интернет-сайте. Каталоги отображают и описывают товар и включают в себя форму заказа по почте, номер телефона или адрес интернет-сайта для размещения заказов.
Листовки и брошюры не считаются каталогами.
Производитель с прайс-листом не считается продавцом по каталогу.'),
        (5965, 'Прямой маркетинг – комбинированный каталог и розничные продавцы', 'Продавцы, которые также работают с одним или несколькими розничными точками. Этот MCC используется только для транзакций, происходящих по почте, телефону, электронной коммерции или другим не предусматривающим личного контакта с покупателем, даже если товар доставляется в магазин для самовывоза.
В этом MCC исключаются личные продажи, в том числе те, в которых заказ помещен на стол каталога или где-либо еще в магазине в ответ на каталог или другое сообщение прямых продаж.
Все розничные продажи должны проходить с соответствующим розничным MCC.'),
        (5966, 'Прямой маркетинг - исходящий телемаркетинг', 'Торговцы, которые инициируют прямые контакты с потребителями для продажи товаров и услуг. Поставщики исходящих телемаркетингов связываются (а иногда и стимулируют) с потенциальным покупателем по телефону, рекламе, прямой почте (кроме каталога) или другому методу прямого маркетинга, который включает либо бесплатный номер телефона, либо почтовый адрес. Товары, рекламируемые и продаваемые с помощью таких методов, могут включать косметику, продукты медицинского назначения, витамины и недвижимость с долевым сроком.
Для благотворительных организаций, которые запрашивают взносы любыми способами, в том числе через исходящий телемаркетинг, используется MCC 8398.'),
        (6536, 'Денежные переводы с карты на карту – зачисление (внутри страны)', 'Определяет операции, при которых держатель карты получает средства за счет перевода на карту Mastercard, Maestro или Cirrus, на стороне получателя (кредитовая часть перевода)'),
        (5967, 'Прямой маркетинг – входящий телемаркетинг', 'Торговцы, которые предоставляют одну или несколько служб аудиотекста или видеотекста, к которым можно получить доступ по телефону, факсу или по Интернету. Владелец карты инициирует контакт с торговцем и все последующие транзакции. Этот MCC применяется к информационным услугам, предлагаемым по телефону или в Интернете, а также к товарам, которые могут быть проданы через такие службы. Информационные услуги предоставляются за вознаграждение и могут включать в себя опросы, лотереи, чаты для взрослых и развлечения, спортивные результаты, котировки на фондовом рынке, показания гороскопа или другие аудиотексты или видеотексты, которые потребители слушают, просматривают или участвуют в них.
Для поставщиков электронного доступа к доске объявлений или онлайн-сервисам, предоставляемым через компьютеры, используется MCC 4816.'),
        (5968, 'Прямой маркетинг – Продажа по подписке', 'Точки, которые продают продукты или услуги по подписке через прямую почтовую рассылку, телефон, интернет или другой метод прямого маркетинга, который включает в себя бесплатный номер, почтовый адрес, адрес электронной почты или URL-адрес веб-сайта. Такие точки могут предлагать серии продуктов (например, одну книгу в месяц на один год) или ежегодное обновление одного продукта. Счет владельцу карты выставляется за продукт или услугу на постоянной или периодической основе (например, один раз в месяц или два раза в год), а предоставление продукта или услуги продолжается до тех пор, пока не закончится подписка или серия, или владелец карты или продавец не расторгнет соглашение. Этот MCC включает в себя книжные клубы, подписку на журналы и газеты, аудиоклубы, продающие записи, кассеты и компакт-диски (CD), клубы с видеокассетами и цифровыми видеодисками (DVD), подписки на коллекционные серии (например, серии марок, монет, книг, таблички, фарфоровые рисунки или тарелки), подписки на журналы, подписки на продукты для здоровья, подписки на косметику, подписки на витамины.'),
        (5969, 'Прямой маркетинг – другие торговые точки прямого маркетинга (нигде более не классифицированные)', 'Торговые точки, которые продают товары и услуги с помощью различных методов прямого ответа, в которых включена форма заказа или адрес или номер телефона для размещения заказа по почте, телефону или факсу. Эти торговцы часто используют методы массового маркетинга, такие как брошюры, объявления с прямым ответом на радио и телевидении, а также телевизионные «рекламные ролики» (рекламные ролики с расширенной длиной с форматом ток-шоу). Такие торговцы обычно предлагают только один или два продукта на рекламу, такие как кухонные столовые приборы, бытовые гаджеты, товары для снижения веса, спортивное оборудование, косметику, специальные записи или книги, которые доступны только через телевизионную рекламу. Это включает рекламу в газетах, продаваемую по методам прямого маркетинга.
Для билетов, заказанных по телефону, но оплаченных в билетной кассе, используется MCC 7922.
Исключаются розничные торговцы лицом к лицу, которые иногда принимают заказы на почту или телефон для удобства клиентов.
Для розничных продаж, проводимых через интернет-сайт, используется наиболее подходящий розничный MCC с TCC T.'),
        (5970, 'Магазины художественных и ремесляных изделий', 'Розничные торговцы которые продают материалы, оборудование, и  т.п., которое используется для создания художественных и ремесляных  изделий и т.п. вещей. Товары для распродажи  могут включать  все типы картин (живописи), краски, шелковые цветы, ручки, карандаши, бумагу, конструкторы для сборки, пряжу, штуковины для изготовления конфет, декоративные резиновые штампы, и.т.п. вещи. Для торговцев которые специализируются на продаже  ткани, выкроек, принадлежностей  для шитья, используют МСС 5949.'),
        (5971, 'Галереи и художественные посредники', 'Розничные торговцы, которые продают  художественные работы, такие как картины, фотографии, скульптуры. Распродажа может проводиться  прямо с посредниками, художниками, в галереях.'),
        (5972, 'Магазины монет и марок (филателические и нумизматические)', 'Розничные торговцы, которые продают почтовые марки, монеты и подобные аксессуары для коллекций или сбережений.
Для распродажи почтовых марок (почта США), используется МСС 9402.
Для почты и почтовых услуг, которые тоже используют почтовые марки, используется МСС 7399.'),
        (5973, 'Магазины религиозных товаров', 'Розничные торговые точки, которые продают такие товары как статуэтки, открытки, религиозные штучки, иконы, картины, книги и кассеты,  декоративные  вещи.
Для торговых точек, которые первоначально продают религиозные книги, используют МСС 5942.'),
        (5975, 'Слуховые аппараты – продажа, сервис, снабжение', 'Розничные торговые точки, которые продают слуховые аппараты и связанные с ними материалы, а также могут или не могут также проводить ремонтные работы.
Для точек, которые в основном проводят ремонтные работы, используйте MCC 7699.'),
        (5976, 'Ортопедические товары', 'Розничные торговые точки, которые продают различные ортопедические товары, протезы и которые также могут заниматься их починкой. Товары для продажи могут включать костыли, палочки и др. вспомогательные детали для передвижения, эластичный трикотаж, абдоминальные поддержатели, обвязки, растяжки, инвалидные коляски, и т.п.
Для торговых точек, которые первоначально занимаются починкой подобных товаров, используется МСС 7699.'),
        (5983, 'Поставщики топлива – уголь, мазут, сжиженная нефть, древесина', 'Розничные торговые точки, которые продают мазут, древесину, уголь, сжиженную нефть, авиационное топливо, топочный мазут или газ-пропан. Такие точки могут или не могут продавать бензин для потребительского использования в автомобилях.
Для точек, которые в основном продают автомобильный бензин, используется MCC 5541 или MCC 5542, если это необходимо.'),
        (5992, 'Флористика', 'Розничные торговые точки, которые продают обрезанные цветы, всякие цветочные приспособления, саженцы. Такие торговые точки могут  или не могут также продавать разные товары типа гелиумных воздушных шаров. Для торговцев, продающих первоначально семена, шары, парники, инвентарь, используют МСС 5261.'),
        (5993, 'Табачные магазины', 'Розничные торговые точки, которые продают табак, сигареты, сигары, трубки и все курительные аксессуары.'),
        (5994, 'Дилеры по продаже печатной продукции', 'Розничные торговые точки, которые продают газеты, журналы и другую периодику.
Для подписки на газеты используется МСС 5968.'),
        (5995, 'Зоомагазины', 'Розничные торговые точки, которые продают домашних питомцев (собачки, кошки, птички, рыбки, рептилии, хомячки, кролики), еду для них и все связанные с ними аксессуары.'),
        (5996, 'Плавательные бассейны – продажа и снабжение', 'Розничные торговые точки, которые продают домашние бассейны (наземные или сборные надземные бассейны), курорты, гидромассажные ванны, гидромассажные ванны и принадлежности для бассейна. Такие точки могут или не могут также предоставлять услуги по установке и техническому обслуживанию.
Для точек, которые в первую очередь предоставляют услуги по установке, используется MCC 1799. Эти торговцы также могут или не могут предоставлять услуги по ремонту.
Для точек, которые в первую очередь предоставляют услуги по ремонту, используется MCC 7699.'),
        (5997, 'Магазины электрических бритвенных принадлежностей – распродажа и услуги.', 'Розничные торговые точки, которые продают электрические бритвенные принадлежности, которые также могут или не могут предоставлять услуги по ремонту.
Для торговых точек, которые изначально обеспечивают услугами по ремонту, используют МСС 7699.'),
        (5998, 'Магазины палаток и навесов', 'Розничные точки, которые продают палатки и навесы для дома или бизнеса. Товары для продажи могут включать палатки для кемпинга, тентовые палатки и преднастроенные или изготовленные на заказ оконные или дверные тенты для дома или бизнеса. Такие точки могут или не могут также оказывать услуги по ремонту.
Для точек, которые в первую очередь предоставляют услуги по ремонту, используется MCC 7699.'),
        (5999, 'Различные магазины и специальные розничные магазины', 'Розничные торговые точки, которые продают уникальные или специализированные товары, которые не попадают ни под какое МСС описание. Этот МСС код должен быть использован только когда этот товар не подходит под другие МСС. Примерами могут послужить специальные магазины, которые торгуют картами и атласами, льдом, дистиллированной водой, аксессуары для магии, вечеринок, красоты, и т.п. вещи.'),
        (6009, 'МФО', 'Погашение займов микрофинансовых организаций'),
        (6010, 'Финансовые учреждения – выдача наличных в кассе', 'Используется для определения транзакций лично, когда владелец карты использует карту в кассе, чтобы получить наличные.

Для получение денежных средств в автоматических устройствах, типа банкоматов, используется МСС 6011.'),
        (6011, 'Финансовые учреждения – снятие наличных автоматически', 'Используется для определения операций выдачи наличных денежных средств и нефинансовых операции в банкоматах клиентов международных платежных систем.

Для снятий наличных, совершаемых в кассе финансовых учреждений, используется MCC 6010.'),
        (6537, 'Денежные переводы с карты на карту – зачисление (между странами)', 'Определяет операции, при которых держатель карты получает средства за счет перевода на карту Mastercard, Maestro или Cirrus, на стороне получателя.'),
        (6538, 'Денежные переводы с карты на карту – списание', 'Определяет операции, при которых держатель карты использует карту для перевода на другую карту Mastercard (дебетовая часть перевода)'),
        (6539, 'Транзакция по финансированию (исключая MoneySend)', null),
        (7513, 'Прокат аксессуаров для трэйлеров и грузовиков.', 'Лизинг оборудования для грузовиков, фургонов, трэйлеров.'),
        (6012, 'Финансовые учреждения – торговля и услуги', 'Покупка товаров или услуг в финансовых учреждениях. Такими товарами и услугами могут быть чеки и другие финансовые продукты, рекламные товары, сборы за предоставление кредита и плата за услуги финансового консультирования, пополнение счета преодоплаченных карт. По документам Visa этот MCC также должен использоваться для погашения долгов, займов или остатка по кредитной карте держателем карты в финансовом учреждении. Также этот MCC используется при оплате услуг микрофинансовых организаций.

Для оплаты услуг по ценным бумагам и брокерским операциям или других соответствующих расходов используется MCC 6211.

Для денежных выплат используется MCC 6010 для очных операций и MCC 6011 для транзакций, которые происходят в банкоматах.'),
        (6022, 'Financial Institution (RCL Internal)', 'Описание не найдено. Скорее всего внутренний код какой-то организации, но есть в списках исключений некоторых банков.'),
        (6023, 'State Banks (RCL Internal)', 'Описания не найдено. Скорее всего внутренний код какой-то организации, но есть в списках исключений некоторых банков.'),
        (6025, 'National Banks (RCL Internal)', 'Описание не найдено. Скорее всего внутренний код какой-то организации, но есть в списках исключений некоторых банков.'),
        (6026, 'National Banks Non Federal (RCL Internal)', 'Описание не найдено. Скорее всего внутренний код какой-то организации, но есть в списках исключений некоторых банков.'),
        (6028, 'Unincorporated Private Banks (RCL Internal)', 'Описание не найдено. Скорее всего внутренний код какой-то организации, но есть в списках исключений некоторых банков.'),
        (6050, 'Квази-Кэш – Финансовые учреждения', 'Покупка чеков, иностранной валюты, пополнение электронных кошельков и другие квази-кэш операции в финансовых учреждениях'),
        (6051, 'Квази-Кэш – Нефинансовые учреждения', 'Покупка чеков, иностранной валюты, пополнение электронных кошельков, торговых счетов и другие квази-кэш операции в нефинансовых учреждениях'),
        (6211, 'Услуги брокеров на рынке ценных бумаг', 'Точки, которые покупают и продают ценные бумаги, акции, облигации, товары и фонды'),
        (6300, 'Услуги страховых компаний', 'Розничные торговые точки, которые продают все виды личных или коммерческих страховых полисов, включая страхование автомобилей, жизни, здоровья, прав собственности на недвижимость, медицинские и стоматологические страховки, страхование для домовладельцев и арендаторов, страхование здоровья домашних животных, страхование от наводнений, пожаров или землетрясений.

Для точек, которые продают страховые продукты и услуги с использованием методов прямого маркетинга, используется MCC 5960.'),
        (6381, 'Страховые премии', 'В справочниках платёжных систем не найдено, но есть в исключениях некоторых банков. По некоторым данным код уже не используется.'),
        (6399, 'Страхование – нигде более не классифицированные', 'По некоторым данным код не используется'),
        (6513, 'Агенты недвижимости и менеджеры - Аренда', 'Сборы, взимаемые точками, занимающихся арендой и управлением жилой и коммерческой недвижимостью, такими как агенты по недвижимости, брокеры и менеджеры, а также услуги по аренде квартир. Такие сборы могут включать плату за управление, комиссионные за аренду недвижимости и платежи за аренду недвижимости.'),
        (6529, 'Удалённое пополнение предоплаченной карты - Финансовые организации', 'Используется TSYS'),
        (6530, 'Удалённое пополнение предоплаченной карты - Торговая точка', 'Используется TSYS'),
        (6531, 'Оплата услуг – денежные переводы', null),
        (6532, 'Платежная операция - финансовое учреждение', 'Этот MCC может использоваться только для идентификации Платежных операций. Платежная транзакция позволяет владельцам карт Mastercard переводить средства на счет Mastercard. Платежная транзакция не отменяет предыдущую транзакцию покупки Mastercard и должна быть авторизована эмитентом.'),
        (6533, 'Платежная операция - продавец', 'Этот MCC может использоваться только для идентификации Платежных операций. Платежная транзакция позволяет владельцам карт Mastercard переводить средства на счет Mastercard. Платежная транзакция не отменяет предыдущую транзакцию покупки Mastercard и должна быть авторизована эмитентом.'),
        (6534, 'Денежный перевод - финансовое учреждение', null),
        (6535, 'Права требования на ценности — Финансовые организации', 'Описания нет нигде, но код встречается в исключениях многих банков'),
        (6540, 'Пополнение небанковских предоплаченных карт, счетов', 'Определяет транзакцию, которая осуществляется точкой, предоставляющей любую из следующих услуг:
• Услуга, в которой средства доставляются или предоставляются другому лицу, не являющемуся владельцем карты;
• Пополнение счета пользователя, если эти средства не будут использованы для азартных игр, покупки табака и лекарств и т.п.;
• Пополнение предоплаченных карт.'),
        (6611, 'Переплата (авансовые платежи)', 'Код не найден в документациях платежных систем, но есть в списках исключений некоторых банков и собственных списках кодов некоторых банков США.'),
        (6760, 'Облигации сберегательного займа', 'Код не найден в документациях платежных систем, но есть в списках исключений некоторых банков и собственных списках кодов некоторых банков США.'),
        (7011, 'Отели и мотели - нигде более не классифицированные', 'Заведения размещения, для которых не был определен уникальный MCC, включая гостиницы, курорты, коттеджи, коттеджи и общежития.'),
        (7012, 'Тайм-шер', 'Розничные продавцы, которые продают, арендуют и арендуют недвижимость в тайм-шер и организуют обмен кондоминиумами в тайм-шер.'),
        (7032, 'Рекреационные и спортивные лагеря', 'Торговцы, которые управляют детскими лагерями, лагерями отдыха и спортивными лагерями. Примеры таких лагерей включают летние лагеря для мальчиков и девочек, рыболовные и охотничьи лагеря, а также учебные или спортивные лагеря.
Для кемпингов, используемых для палаточного или прицепного кемпинга, используется MCC 7033.'),
        (7033, 'Кемпинги и трейлер-парки', 'Торговцы, которые предоставляют ночные или краткосрочные кемпинги для рекреационных автомобилей, прицепов, кемперов или палаток. Такие кемпинги могут быть расположены в государственных парках, или находиться в частной собственности и эксплуатироваться, и могут включать или не включать водные и электричество для рекреационных транспортных средств.'),
        (7210, 'Услуги по уборке и прачечной', 'Торговцы, которые управляют парами или другими видами прачечных для коммерческих предприятий или физических лиц. Еженедельно или ежемесячно эти торговцы предоставляют отмытые предметы, такие как униформа, халаты, фартуки, столовое белье, постельное белье и полотенца. Этот MCC также включает в себя обслуживание подгузников, предлагающие вывоз из дома, уборку и доставку.'),
        (7211, 'Услуги прачечной - семейные и коммерческие', 'Точки, которые предоставляют стиральные и сушильные машины самообслуживания для общего пользования, в том числе услуги прачечной с платой за вес. Такие прачечные самообслуживания обычно известны как лондроматы (прачечные-автоматы), но их также можно найти в общежитиях, многоквартирных домах или других подобных местах.'),
        (7216, 'Химчистка', 'Точки, которые предоставляют услуги химчистки для частных лиц или предприятий, и могут предлагать ограниченные услуги по переделке. Обычно в химчистках чистят одежду, шторы, свадебные платья и постельные принадлежности.'),
        (7217, 'Чистка ковров и обивки мебели', 'Точки, которые чистят для частных лиц и предприятий ковры, ткани и обивку мебели.

Для точек, которые предоставляют услуги замены обивки, используется MCC 7641.'),
        (7221, 'Фотостудии', 'Торговцы, которые производят фотосъемку или видеосъемку для широкой публики, в том числе фотографии детей в школах или свадебные фото и видео.
Для продавцов, которые предоставляют услуги по разработке и печати плёнки, используется MCC 7395.'),
        (7230, 'Парикмахерские и салоны красоты', 'Торговцы, которые предоставляют персональные услуги по уходу за волосами, такие как стрижка волос, укладка волос, окраска волос и наращивание волос. Парикмахерские и салоны красоты могут также выполнять маникюр и педикюр и продавать ограниченный ассортимент средств по уходу за волосами.'),
        (7251, 'Чистка шляп, ремонт и полировка обуви', 'Точки, которые ремонтируют обувь и предоставляют услуги по чистке и полировке обуви, а также точки, которые предоставляют услуги по чистке и блокировке шляп.'),
        (7261, 'Ритуальные услуги и крематории', 'Точки, которые предоставляют услуги по подготовке похорон, кремации и проводят похороны. К таким точкам относятся морги, крематории, похоронные дома и похоронные бюро.

Для услуг кремации и захоронения животных используется MCC 7299.'),
        (7276, 'Служба налоговой подготовки', 'Торговцы, которые исключительно предоставляют услуги по подготовке декларации по налогу на прибыль без аудита, бухгалтерского учета или бухгалтерских услуг.
Для продавцов, предоставляющих услуги аудита, бухгалтерского учета или бухгалтерского учета в дополнение к услугам по подготовке налоговой декларации, используйте MCC 8931.'),
        (7277, 'Долги, брак, личные вопросы - консультирование', 'Точки, предоставляющие разнообразные консультационные услуги, такие как консультирование по вопросам задолженности и финансов, консультирование по вопросам брака, консультирование по вопросам семьи, консультирование по вопросам злоупотребления алкоголем и наркотиками и другие личные консультации.
Для точек, которые предоставляют юридические услуги, используется MCC 8111.'),
        (7278, 'Услуги покупок/шоппинга', 'Торговые точки которые предлагают услуги по продаже товаров как для частных, так и для корпоративных лиц. Напрямую эти точки товары не продают, а лишь оказывают услуги посредника по продаже за определённую плату.'),
        (7280, 'Пациент больницы – вывод личных средств', 'Код не найден в документах платёжных систем, но есть в некоторых других документах и списках исключений некоторых банков'),
        (7296, 'Сдача в аренду костюмов, униформы, простой одежды', 'Торговые точки, сдающие в аренду костюмы, смокинги, одежду, униформу и другие типы верхней одежды и аксессуары.'),
        (7297, 'Массажные салоны', 'Терапевтические приемные, предлагающие услуги массажа. Некоторые из них могут также оказывать индивидуальные процедуры, такие как массаж лица и ароматерапию.'),
        (7298, 'Салоны красоты и здоровья', 'Торговцы, обычно известные как дневные курорты, предоставляющие разнообразные личные или терапевтические услуги без ночевки. Такие услуги могут включать уход за лицом, массаж, грязевые ванны, травяные обертывания, сеансы загара, гидромассажные ванны, паровые ванны, индивидуальные программы упражнений, консультирование по вопросам питания, а также консультирование и укладка волос и макияжа, а также учебные занятия, такие как аэробика, Контроля, приготовления пищи и занятий спортом.
Для спа-услуг, предлагаемых в рамках плана пакета, который включает проживание в торговом представительстве, используется MCC 7011.'),
        (7299, 'Различные услуги - нигде более не классифицированные', 'Точки, предоставляющие личные услуги, которые не подпадают какой-либо другой MCC.

Примерами таких услуг являются пансионы для животных, бани, услуги сиделок, услуги горничной, уход за животными и питомники, обучение или разведение животных, агенты по недвижимости, брокеров и менеджеров, пирсинг и татуировка, услуги по очистке воды, таксидермисты, свадебные часовни, а также кремация и погребение животных.
Для агентов по недвижимости, брокеров и менеджеров, а также услуги по аренде квартир, используется MCC 6513.'),
        (7311, 'Рекламные услуги', 'Точки, которые занимаются подготовкой рекламы (копирайтинг, художественные работы, графика и другие творческие работы) и размещают рекламу в периодических изданиях, газетах, на радио, телевидении или других рекламных носителях для клиентов по договору или на платной основе.
Также включены другие виды рекламы, такие как реклама и надписи в небе, распространение купонов и распространение образцов.'),
        (7321, 'Кредитные бюро', 'Точки, предоставляющие услуги по предоставлению кредитных отчетов, такие как определение кредитоспособности, расчетно-клиринговые и другие сопутствующие услуги. Этот MCC не включает коллекторские агентства.'),
        (7322, 'Агентства взыскания долгов', null),
        (7332, 'Услуги синьки и фотокопирования', null),
        (7333, 'Коммерческое искусство, графика, фотография', 'Точки, которые предоставляют коммерческое искусство или услуги графического дизайна для рекламных агентств, издателей и других предприятий. Такие услуги могут включать в себя дизайн шелкографии, кинопроизводство и слайд-фильм, графический дизайн, коммерческое искусство и иллюстрации.
Для продавцов, которые предоставляют фото, видео и портретную съемку для личных целей, используется MCC 7221.'),
        (7519, 'Прокат домиков на колесах, аксессуары к наземному транспорту.', 'Прокат прицепов, вагончиков, фургонов, кемпов, тентов для грузовиков как на короткие сроки, так и на большие.'),
        (7339, 'Услуги стенографии и секретарского дела', 'Торговцы, предоставляющие стенографические, секретарские и другие канцелярские услуги, такие как обработка текстов, машинопись, редактирование, письмо, корректура, составление резюме и услуги по составлению судебных отчетов.
Для служб занятости, которые в основном занимаются заполнением секретарских должностей, используется MCC 7361.'),
        (7342, 'Дезинсекция, дезинфекция и дератизация', 'Точки, которые предоставляют услуги по борьбе с вредителями, уничтожению, дезинфекции и фумигации, включая уничтожение термитов, насекомых и грызунов.'),
        (7349, 'Уборка и техническое обслуживание зданий и помещений', 'Точки, которые предоставляют услуги по уборке и, обслуживанию зданий, такие как мытье окон, полов, услуги школьного сторожа, уборка офисов и помещений по договору или на платной основе.'),
        (7361, 'Агентства по трудоустройству, временные справочные службы', 'Торговцы, которые предоставляют услуги по трудоустройству для работодателей или лиц, ищущих работу с постоянными или временными должностями.'),
        (7372, 'Программирование, обработка данных, проектирование интегрированных систем', 'Услуги программирования, проектирования систем и обработки данных на основе контракта и платы. Такие услуги могут включать проектирование и анализ компьютерного программного обеспечения, модификацию систем и программного обеспечения, ввод или обработку данных и обучение использованию программного обеспечения.'),
        (7375, 'Информационно-поисковые услуги', 'Провайдеры информационных технологий, информации о базах данных, услуги интернет-провайдера.'),
        (7379, 'Ремонт компьютеров', 'Техническое обслуживание компьютерного оборудования. Консультации специалиста в этой области. Внедрение баз данных, восстановление информации с пленки, дискет, анализ компьютерных требований.'),
        (7389, 'Бизнес услуги – нигде более не классифицированные', 'MCC код не найден, но данный код есть в справочниках SIC-кодов и входит в списки исключений некоторых российских банков. Судя по всему является аналогом MCC 7399.'),
        (7392, 'Услуги по консультированию, управлению и связям с общественностью', 'Торговцы, которые предоставляют консультации и помощь в управлении частными, некоммерческими и общественными организациями по договору или на платной основе. Предоставляемые услуги могут включать стратегическое и организационное планирование, финансовое планирование и бюджетирование, разработку маркетинговых целей, планирование информационных систем, разработку кадровой политики, планирование процедур и услуги по связям с общественностью.'),
        (7393, 'Детективные агентства, охранные агентства, службы безопасности, включая бронированные автомобили, сторожевых собак', 'Точки, предоставляющие охранные устройства и службы безопасности. Примерами таких устройств и услуг являются бронированные автомобили, системы безопасности (установка, мониторинг и обслуживание), частные следователи, сторожевые собаки и детекторы лжи.'),
        (7394, 'Аренда оборудования и лизинговые услуги, аренда мебели, прокат инструментов', 'Торговцы, которые арендуют и сдают в лизинг оборудование, инструменты, мебель, бытовую технику и оргтехнику (за исключением пишущих машинок).
Для проката пишущих машинок используется MCC 5978.'),
        (7395, 'Фотостудии, фотолаборатории', 'Печать фотографий, проявка плёнки, ретуширование, фотоувеличение, весь спектр фотомонтажа и ремастеринга. Продажа рамок, фотоальбомов, фотопленки, фотоаппаратов.'),
        (7399, 'Бизнес услуги – нигде более не классифицированные', 'Точки, которые предоставляют коммерческие и торговые услуги, которые обычно не считаются профессиями. Этот MCC должен использоваться, только если другой более конкретный MCC не описывает бизнес точки. Примерами таких бизнес-услуг являются издательские компании, компании по управлению конференциями, организаторы совещаний, компании по проведению семинаров, слесари, почтовые и упаковочные услуги (включая продажу марок), службы обработки сообщений и пейджинга, а также услуги по управлению отходами.'),
        (7511, 'Стоянка грузового транспорта', null),
        (7512, 'Агентства по прокату автомобилей - нигде более не классифицированные', 'Агентства по аренде автомобилей, для которых не назначен уникальный MCC.'),
        (7531, 'Кузовной ремонт автомобилей', 'Точки, которые выполняют кузовные работы. Такие точки также могут или не могут покрасить автомобили в связи с ремонтом кузова.

Для точек, которые в основном красят автомобили, используется MCC 7535.

Для точек, которые выполняют ремонтные работы, кроме ремонта кузова, используется MCC 7538.'),
        (7534, 'Шиномонтаж и вулканизация', 'Этот MCC определяет точки, которые продают, устанавливают, ремонтируют или восстанавливают старые шины.

Для точек, которые предоставляют другие виды услуг по ремонту автомобилей, используется MCC 7538.
Для точек, которые в основном продают новые автомобильные шины, используется MCC 5532.'),
        (7535, 'Покраска автомобилей', 'Точки, которые красят и полируют исключительно автомобили, в том числе те, которые специализируются на реставрации старинных автомобилей, а также делают покраску на заказ.'),
        (7538, 'Автосервисы', 'Точки, которые проводят ремонт автомобилей и общее обслуживание, такое как обслуживание двигателей, тормозной системы, системы кондиционирования воздуха, глушителей, передней части и рамы, топливной системы, карбюраторов, радиаторов, ветровых стекол и окон, а также электроники. В этот MCC входят точки быстрой замены масла и продавцы смазочных материалов. Такие автосервисы также могут выполнять или не выполнять услуги по шиномонтажу.'),
        (7542, 'Автомойки', 'Точки, которые моют, чистят воском и полируют автомобили, в том числе щеточные мойки, ручные мойки и мойки самооблуживания.'),
        (7549, 'Услуги буксировки и эвакуации', 'Точки, которые предоставляют услуги буксировки транспортных средств.'),
        (7622, 'Ремонт электроники', 'Ремонт электроники, такой, как радиоприёмников, телевизоров, аудио аппаратуры, СD-проигрывателей, компьютеров.'),
        (7623, 'Ремонт кондиционеров и холодильников', 'Точки, которые обслуживают и ремонтируют бытовые и коммерческие кондиционеры, а также холодильные системы.
Для точек, обслуживающих и ремонтирующих небольшие бытовые приборы, используется MCC 7629.
Для точек, которые обслуживают и ремонтируют большие бытовые приборы, используется MCC 7699.'),
        (7629, 'Ремонт электрооборудования и малой бытовой техники', 'Точки, которые обслуживают и ремонтируют бытовые и коммерческие электрические компоненты и небольшие приборы, такие как микроволновые печи, тостеры, а также точки, которые обслуживают и ремонтируют электрические офисные машины, за исключением пишущих машин.
Для точек, специализирующихся на ремонте пишущей машинки, используется MCC 5978.
Для точек по ремонту электроники используется MCC 7622.
Для точек, которые ремонтируют большие бытовые приборы, используется MCC 7699.'),
        (7631, 'Центры ремонта часов и чистки ювелирных изделий', 'Торговые точки по ремонту часов, чистке ювелирных изделий, украшений.'),
        (7641, 'Обивка, ремонт и отделка мебели', 'Точки, которые занимаются обивкой, ремонтом и отделкой мебели, включая реставраторов антикварной мебели.'),
        (7692, 'Ремонт с помощью сварки', 'Торговые точки специализирующиеся по ремонту с помощью сварки.'),
        (7699, 'Ремонтные услуги – нигде более не классифицированные', 'Агенства по ремонтным работам, которые не могут быть классифицированы как какие-либо конкретные. Некоторые из этих агенств могут предлагать отдельные услуги по ремонту бытовой техники (к примеру: сушилки, посудомоечные машины, нагреватели воды), велосипедов, спортивных тренажеров, слуховых аппаратов, протезов, музыкальных инструментов, палаток и подвесок, видеокамер и фотоаппаратов, газонокосилок, чемоданов и кожаных изделий. Также включают в себя центры по чистке и обслуживанию котлов и дымовых труб.'),
        (7800, 'Государственные лотереи (только США)', 'Государственные органы, которые управляют лотереями, а также занимаются продажей лотерейных билетов или акций в Интернете или в офисе напрямую или через назначенных агентов.

Использование этого MCC ограничено точками, расположенными в регионе США.'),
        (7801, 'Азартные игры в интернете (только США)', 'Торговцы, получившие лицензию в соответствии с действующим законодательством или правилами, чтобы управлять системой или платформой интернет-азартных игр, принимающей размещение ставок.
Использование этого MCC ограничено точками, расположенными в США'),
        (7802, 'Лошадиные / собачьи бега (только США)', 'Торговцы, получившие лицензию в соответствии с применимым законодательством или правилами для проведения в отношении лошадей или собачьих гонок или пари-мутул, или в обоих случаях.
Использование этого MCC ограничено продавцами, расположенными в США'),
        (7829, 'Производство и распространение кинофильмов и видеокассет', 'Оптовые производители и распространители образовательных и промышленных фильмов и видеороликов для выставок и продаж, в том числе рекламных роликов и учебных фильмов.

Для предприятий розничной торговли, которые сдают в аренду видеокассеты потребителям, используется MCC 7841.'),
        (7832, 'Кинотеатры', 'Точки, которые управляют кинотеатрами, в том числе под открытым небом (drive-in). Такие продавцы продают билеты и напитки и могут предлагать или не предлагать предварительное бронирование билетов по телефону.'),
        (7833, 'Экспресс-оплата – Кинотеатр', null),
        (7841, 'Видеопрокат', 'Точки которые сдают в аренду видеокассеты, CD, DVD и видеоигры для домашнего использования. Такие продавцы могут продавать или не продавать ограниченный выбор использованных видео, напитки, конфеты, закуски и чистые кассеты.'),
        (7911, 'Танцевальные залы, школы и студии', 'Точки, которые управляют танцевальными студиями, танцевальными школами и общественными танцевальными залами или бальными залами, взимающие плату за вход, уроки и прохладительные напитки. Такие точки могут обучать многим видам танца или специализироваться на чечетке и балете, бальных, кадриле и других видах танца.'),
        (7922, 'Театральные продюсеры (кроме кинофильмов), билетные агентства', 'Точки, которые управляют живыми театральными постановками, такими как дорожные компании и летние театральные группы. Сюда также входят услуги, связанные с театральными постановками и концертами, такие как кастинговые агентства, агентства бронирования, декорации, освещение и другое оборудование, а также театральные билетные агентства.

Для кинотеатров используется MCC 7832.'),
        (7929, 'Музыкальные группы, оркестры и прочие развлекательные услуги', 'Точки, которые обеспечивают развлечения кроме театральных постановок, и включают музыкантов, группы, оркестры, комиков и фокусников. Точки, которые специализируются на музыке (живые группы или диджеи) для свадеб или мероприятий, также включены.'),
        (7932, 'Бильярд-клубы', 'Заведения, которые сдают в аренду бильярдные столы для развлечения. Такие точки могут предлагать бильярдные столы, управляемые монетами, или могут арендовать бильярдные столы на игру или по часам. Также могут быть доступны шаффлборд, дартс и другие игры и напитки.'),
        (7933, 'Боулинг', 'Точки, которые используют дорожки для боулинга для развлечения и могут принимать карты за арендную плату, покупки в магазине, напитки.

Точки, расположенные в пределах дорожек для боулинга, которыми управляют отдельно, должны быть классифицированы с соответствующим MCC для этого типа бизнеса. Например, ресторан, расположенный в кегельбане, должен использовать MCC 5812, бар должен использовать MCC 5813.'),
        (7941, 'Атлетические поля, коммерческие виды спорта, профессиональные спортивные клубы, промоутеры спорта', 'Точки, которые управляют и продвигают полупрофессиональные и профессиональные спортивные клубы (такие как бейсбол, баскетбол, футбол, хоккей и футбол), продвигают любительские и профессиональные атлетические события и управляют индивидуальными спортсменами. Сюда также входят спортивные арены и стадионы.'),
        (7991, 'Туристические достопримечательности и выставки', 'Точки, которые управляют туристическими достопримечательностями и выставками для развлечений, таких как экспозиции, ботанические сады, ремесленные шоу, музеи и винодельни.

Для художественных выставок, проводимых в художественных галереях, используется MCC 5971.'),
        (7992, 'Публичные поля для гольфа', 'Торговые точки, которые управляют общественными полями для гольфа. Предлагаемые услуги могут включать в себя оплату за поле, прокат гольф-каров или прокат снаряжения.
Для ресторанов и кафе, расположенных на поле для гольфа, используется MCC 5812.
Для профессиональных магазинов, продающих принадлежности для гольфа и оборудование, используется MCC 5941.
Для загородных клубов и частных полей для гольфа используется MCC 7997.'),
        (7993, 'Принадлежности для видеоигр', 'Торговые точки, которые продают игровые автоматы, оборудование и расходные материалы. Товары для продажи могут включать в себя автоматы с видеоиграми, музыкальные автоматы, пинбольные машины, слот-машины, механические игры и фотокабины.
Для торговых точек, которые управляют видео и игровыми аркадами, используется MCC 7994.'),
        (7994, 'Клубы видеоигр', 'Точки, которые используют интерактивные игры и развлекательные машины, такие как музыкальные автоматы, видеоигры, пинбольные машины, пейнтбол и игры с лазертегом, механические игры и будки мгновенной фотографии.

Для точек, которые продают оборудование для развлечений, используется MCC 7993.

Для точек, работающих с играми, использующими ставки, используется MCC 7995.'),
        (7995, 'Азартные игры', 'Любая транзакция, кроме транзакции через банкомат, включающая в себя размещение ставки, покупку лотерейного билета, распространение ставок, коммерческие игры в полете или покупку фишек или другой ценности, используемой для азартных игр в сочетании с игровой деятельностью, предоставляемой заведения для ставок или пари, такие как казино, ипподромы, карточные салоны, авиакомпании и тому подобное.'),
        (7996, 'Парки аттракционов, карнавалы, цирки, гадалки', 'Торговые точки, которые управляют парками аттракционов и карнавалами, предлагают механические аттракционы, закуски и киоски с едой, игры, выставки животных и развлечения. Также включены цирки, гадалки, астрологи, читатели карт таро и мистики.'),
        (7997, 'Клубы – загородные клубы, членство (отдых, спорт), частные поля для гольфа', 'Торговые точки, которые управляют спортивными и рекреационными объектами, требующими членства. Примерами таких объектов могут быть спортивные и оздоровительные клубы, загородные клубы, частные поля для гольфа, лодочные яхт-клубы, клубы плавания, теннисные клубы, лиги для боулинга, клубы верховой езды, стрелковые клубы и клубы для игры в ракетбол. Такие точки могут или не могут предоставлять услуги массажа и спа-услуги, такие как уход за лицом, консультации по контролю веса, ароматерапия, паровые бани и джакузи.

Для торговцев, которые в основном предоставляют массаж, используется MCC 7297.
Для торговцев, которые в основном управляют спа и связанных с ним услугах, используется MCC 7298.'),
        (7998, 'Аквариумы, дельфинарии, зоопарки и морские парки', 'Торговые точки, которые управляют парками морской жизни и зоопарками для развлечения и образования широкой общественности.
Другие услуги, расположенные на территории, такие как рестораны или магазины подарков, должны быть отнесены к MCC, которые лучше всего описывают этот тип бизнеса.'),
        (7999, 'Услуги отдыха - нигде более не классифицированные', 'Точки, которые предоставляют широкий спектр услуг для отдыха и развлечений, которые не описаны другим, более конкретным MCC. Как правило, эти точки предоставляют услуги, которые требуют активного физического участия, такие как плавание, игра в минигольф, катание на лыжах, мини автомобильные гонки, катание на коньках, скалолазание, катание на скейте и катание на лошадях. Эти точки могут также сдавать в аренду велосипеды, самолеты, мотоциклы, карты и другое спортивное оборудование. Как правило, эти точки не являются клубами и не требуют членства.

Для частных спортивных клубных и клубов отдыха, требующих членство, используется MCC 7997.

При аренде лодок используется MCC 4457. Для общественных полей для гольфа используется MCC 7992.'),
        (8011, 'Врачи – нигде более не классифицированные', 'Лицензированные врачи, занимающиеся общей или специализированной медициной и хирургией, которая не описана другим, более конкретным MCC. К таким точкам относятся пластические и косметические хирурги, дерматологи, психиатры, рентгенологи, ортопеды, педиатры, неврологи, акушеры и гинекологи.'),
        (8021, 'Стоматологи, ортодонты', 'Лицензированные врачи, которые практикуют общую или специализированную стоматологию, а также стоматологическую хирургию.'),
        (8031, 'Остеопаты', 'Лицензированные врачи, которые проводят терапию с использованием нехирургических манипуляций и корректировки костных структур.

Для ортопедов, которые проводят хирургическое лечение, используется MCC 8011.'),
        (8041, 'Мануальные терапевты', 'Медицинские работники, выполняющие лечение позвоночника, опорно-двигательной системы и др.'),
        (8042, 'Оптометристы, офтальмологи', 'Оптометристы - это медицинские работники, которые проверяют и лечат дефекты глаз с помощью корректирующих линз или упражнений. Офтальмологи являются лицензированными врачами, которые специализируются на структурах, функциях и заболеваниях глаз и могут выполнять операции. Эти точки могут или не могут заполнять рецепты для очков или контактных линз или продавать оправы для очков.'),
        (8043, 'Оптика, оптические товары и очки', 'Торговые точки, которые читают предписания по исправления зрения, заказывают линзы, производят и продают очки и контактные линзы, и могут проводить проверку глаза. Оптики могут также продавать различные оптические товары, такие как защитные очки, солнцезащитные очки (обычные и специальные), футляры для очков и контактных линз. Также включены т.н. "Очки за час", располагаемые в торговых центрах.

Для торговых точек, которые проверяют глаза и проводят процедуры при дефектах глаз с использованием операционных или хирургических методов, используется MCC 8042.'),
        (8044, 'Оптические товары и очки (TSYS)', 'Торговые точки, которые читают предписания по исправления зрения, заказывают линзы, производят и продают очки и контактные линзы, и могут проводить проверку глаза. Оптики могут также продавать различные оптические товары, такие как защитные очки, солнцезащитные очки (обычные и специальные), футляры для очков и контактных линз. Также включены т.н. "Очки за час", располагаемые в торговых центрах.

Код используется системой TSYS. Для Visa и Mastercard используется MCC 8043.'),
        (8049, 'Ортопеды', 'Лицензированные врачи, специализирующиеся на диагностике и лечении болезней ноги'),
        (8050, 'Услуги персонального ухода', 'Точки, которые предоставляют стационарные услуги по уходу за больными и связанные со здоровьем персональные услуги. Из-за их психического или физического состояния пациентам в этих учреждениях могут потребоваться введения лекарств и лечения или наблюдения за самостоятельными лекарствами в соответствии с указаниями врача. Этот MCC включает в себя дома для выздоравливающих, дома престарелых, помещения для хосписа и учреждения для престарелых, которые обеспечивают длительный уход и обычно не выполняют хирургическое вмешательство.'),
        (8062, 'Больницы', 'Больницы, которые оказывают диагностические услуги, обширное медицинское лечение, включая хирургическое вмешательство и другие больничные услуги, а также услуги постоянного ухода для больных и раненых. Включает психиатрические больницы, детские больницы и родильные дома.
Для ветеринарных клиник или больниц для животных используется MCC 0742.'),
        (8071, 'Стоматологические и медицинские лаборатории', 'Точки, которые предоставляют услуги медицинских и стоматологических специалистов. Включает медицинские лаборатории, которые выполняют биологический анализ, анализ крови, анализ мочи, рентгенологический анализ, бактериологический анализ и диагностический анализ, а также стоматологические лаборатории, которые делают зубные протезы, искусственные зубы, коронки и мосты и индивидуальные ортодонтические приборы.'),
        (8099, 'Медицинские работники, медицинские услуги – нигде более не классифицированные', 'Медицинские специалисты, которые не описаны другим, более конкретным MCC. Примеры включают физиотерапевтов, профессиональных терапевтов, психологов и других медицинских работников, таких как банки крови, клиники по планированию семьи, центры химической зависимости, службы тестирования слуха, лицензированные массажисты, хирургические клиники для замены волос и клиники спортивной медицины.'),
        (8111, 'Адвокаты, юридические услуги', 'Точки, которые предоставляют юридические консультации и услуги. Такие точки могут специализироваться в одной области судебных процессов, таких как развод или банкротство или предоставлять полный спектр юридических услуг.'),
        (8211, 'Начальная и средняя школы', 'Начальные и средние школы, обеспечивающие академическое обучение от детского сада до колледжа, включая государственные, частные, приходские школы, а также школы-интернаты.

Для дошкольных заведений используется MCC 8351.'),
        (8220, 'Колледжи, университеты, профессиональные училища и техникумы', 'Колледжи, университеты, теологические семинарии и профессиональные училища (в том числе стоматологические, юридические, инженерные и медицинские), которые предоставляют академические курсы и предоставляют ученые степени. Требованием для поступления должно быть, по крайней мере, диплом средней школы или его эквивалент.

Для бизнес-школ и секретарских школ используется MCC 8244. Для профессиональных школ используется MCC 8249.'),
        (8241, 'Дистанционные школы', 'Школы заочного обучения, которые предлагают учебные пособия по почте, отправляя студентам уроки и экзамены.'),
        (8244, 'Бизнес / секретарские школы', 'Бизнес и секретарские школы, которые предлагают общие бизнес-тренинги, такие как управление бизнесом, офисные процедуры, обработка текстов, стенография и другие канцелярские навыки. Такие школы предлагают свидетельство об обучении, но не предлагают ученых степеней.

Для профессиональных школ используется MCC 8249.'),
        (8249, 'Профессиональные школы и училища', 'Профессионально-технические училища, которые предлагают обучение и инструктаж по таким специальностям, как сварка, механика, столярные работы, недвижимость и вождение грузовых автомобилей. Такие школы предлагают свидетельство об обучении, но не предлагают ученых степеней.'),
        (8299, 'Образовательные услуги,  нигде более не классифицированные', 'Точки, которые предлагают образовательные услуги, которые не описаны другим, более конкретным MCC. К таким точкам относятся школы, в которых преподают музыку, театр, искусство, кулинарию, лепку, школы каратэ, обучение вождению автомобилей и безопасному движению, обучение полетам, обучение шитью и вязанию и т.д.

Для танцевальных школ используется MCC 7911.

Для музыкальных, театральных и художественных школ, которые предоставляют документы об академической степени, используется MCC 8220.'),
        (8351, 'Услуги ухода за детьми', 'Точки, которые предоставляют услуги по уходу за детьми включая нянь, ясли и детские сады. Такие точки обычно ухаживают за младенцами и дошкольниками, но могут также заботиться о детях старшего возраста и могут предоставлять или не предоставлять образовательные программы.

Для школ используется MCC 8211.'),
        (8398, 'Благотворительные организации и социальные службы - сбор средств', 'Благотворительные (не политические) организации, которые собирают взносы, организации социального обслуживания, которые предоставляют услуги социального обеспечения, группы защиты интересов, общественные организации и агентства здравоохранения.'),
        (8641, 'Гражданские, социальные и братские ассоциации', 'Ассоциации, занимающиеся гражданской, социальной или братской деятельностью. К таким ассоциациям относятся ассоциации и клубы выпускников, клубы повышения квалификации, клубы предпринимателей, общины, братские ложи, братства и клубы, социальные клубы, организации ветеранов и молодежные ассоциации.

Деятельность этих групп может включать сбор политических средств; однако, если это является основной целью организации, используется MCC 8651.'),
        (8651, 'Политические организации', 'Членские организации, которые поддерживают интересы национальных, государственных или местных политических партий или кандидатов, включая политические группы, организованные специально для сбора средств для политической партии или отдельного кандидата.'),
        (8661, 'Религиозные организации', 'Религиозные организации, предоставляющие богослужения, религиозную подготовку или учебу, религиозные мероприятия и сбор средств. Примерами являются церкви, монастыри, мечети, святилища, синагоги и храмы.'),
        (8664, 'Категория неизвестна', 'Код не найден в документации ни одной из платёжных систем, но находится в списках mcc-кодов для категории "Развлечения" у некоторых банков.'),
        (8675, 'Автомобильные ассоциации', 'Такие торговые точки предоставляют своим членам специальные услуги, такие как информация о путешествиях и состоянии дороги, карты и путеводители, а также специальные тарифы в ресторанах, отелях и агентствах по прокату автомобилей. Эти торговые точки часто взимают ежегодный членский взнос.'),
        (8699, 'Членские организации – нигде более не классифицированные', 'Организации или ассоциации, которые не описаны другим, более конкретным MCC. Например, это могут быть исторические клубы, профсоюзы, поэтические клубы, художественные советы и ассоциации арендаторов или кондоминиумов.

Для членства в оздоровительных и спортивных клубах используется MCC 7997.'),
        (8734, 'Испытательные лаборатории (немедицинские)', 'Торговые точки, которые предоставляют немедицинские услуги тестирования другим предприятиям. Примерами таких услуг являются автомобильные испытания, калибровка и сертификация, тестирование продуктов питания, судебно-медицинская экспертиза, тестирование продуктов и загрязнений, а также услуги промышленной рентгенографии.

Для медицинских испытательных лабораторий используется MCC 8071.'),
        (8743, 'Испытательные лаборатории (немедицинские)', 'Торговые точки, которые предоставляют немедицинские услуги тестирования другим предприятиям. Примерами таких услуг являются автомобильные испытания, калибровка и сертификация, тестирование продуктов питания, судебно-медицинская экспертиза, тестирование продуктов и загрязнений, а также услуги промышленной рентгенографии.

Код используется системой TSYS. Для Visa и Mastercard используется MCC 8734.'),
        (8911, 'Архитектурные, инженерные и геодезические услуги', 'Точки, которые предоставляют профессиональные архитектурные, инженерные и земельно-, водно- и воздушные услуги геодезии. Сюда входят проектировщики домов и зданий, инженеры-кораблестроители, инженеры и проектировщики машин, а также инженеры-архитекторы.'),
        (8931, 'Аудит и бухгалтерский учет', 'Точки, предоставляющие услуги бухгалтерского учета, бухгалтерии, выставления счетов, начисления заработной платы и другие сопутствующие аудиторские услуги, включая сертифицированных общественных бухгалтеров (CPA). Такие точки также могут предоставлять услуги по подготовке подоходного налога в дополнение к бухгалтерским и аудиторским услугам.

Для точек, которые предоставляют услуги по подготовке подоходного налога, не предоставляя также аудиторские и бухгалтерские услуги, используется MCC 7276.'),
        (8999, 'Профессиональные услуги - нигде более не классифицированные', 'Точки, занимающиеся традиционными «профессиями», которые предлагают узкоспециализированные услуги, и часто требуют от сотрудников получения дополнительного или специального образования или обучения для предоставления услуг. Этот MCC следует использовать только в том случае, если бизнес торговой точки не описывается другим, более конкретным MCC. К примерам таких точек относятся ипотечные брокеры, исследовательские фирмы, специалисты по финансовому планированию, графические дизайнеры, приглашенные докладчики и преподаватели, судебные стенографисты, оценщики недвижимости, исследовательские фирмы и аукционные дома.'),
        (9034, 'I-Purchasing Pilot', 'Описание не найдено, но код есть в списках исключений некоторых банков.

Используется системой TSYS.'),
        (9211, 'Судебные выплаты, включая алименты и детскую поддержку', 'Местные, региональные и федеральные суды, которые администрируют и обрабатывают судебные издержки (такие как расходы на ведение дел и судебные издержки) и алименты.'),
        (9222, 'Штрафы', 'Государственные органы, которые управляют и обрабатывают местные, региональные и федеральные штрафы и пени, штрафы за нарушение правил эксплуатации транспортных средств и штрафы, налагаемые на сообщество или имущество.'),
        (9223, 'Платежи по залогам и облигациям', 'Торговые точки, которые отправляют залог в судебную систему в качестве гарантии появления обвиняемого'),
        (9311, 'Налоговые платежи', 'Местные, региональные и федеральные организации, которые занимаются финансовым администрированием и налогообложением, включая сбор налогов и штрафов, а также хранение и расходование средств. К таким торговым точкам относятся офисы оценщиков налога на имущество, таможенные бюро и государственные налоговые комиссии.'),
        (9390, 'Госуслуги', 'Операции оплаты государственных услуг на единый казначейский счет (ЕКС) при организации платежей в Федеральной государственной информационной системе «Единый портал государственных и муниципальных услуг (функций)» (ГИС ЕПГУ)'),
        (9399, 'Государственные услуги - нигде более не классифицированные', 'Торговые точки, предоставляющие правительству общие вспомогательные услуги, такие как услуги управления персоналом, аудит, закупки и услуги по управлению зданиями, которые не описаны другим, более конкретным MCC кодом. Примерами могут служить комиссии по гражданским правам и государственной службе, бухгалтерии сектора государственного управления, офисы общего обслуживания, государственные органы снабжения, полицейские, пожарные и автотранспортные службы, а также национальные, государственные и городские парки.

Для транзакций по программе внутригосударственных трансфертов (IGOTS) используется MCC 9405.

Для государственных университетов и колледжей используется MCC 8220.

Торговые точки, участвующие в продаже товаров или услуг правительству, должны использовать оптовый MCC, который лучше всего описывает бизнес.'),
        (9400, 'Перевод на специальный счёт оператора финансовой платформы', 'Перевод денежных средств с банковских счетов (вкладов) физического лица на специальный счёт оператора финансовой платформы (ОФП) для зачисления в пользу такого физического лица'),
        (9401, 'I-Purchasing Pilot', 'Описание не найдено, но код есть в списках исключений некоторых банков.

Используется системой TSYS.'),
        (9402, 'Почтовые услуги – только государственные', 'Государственные почтовые отделения, включая местные почтовые отделения. Предоставляемые услуги включают в себя прием и обработку посылок и почты для доставки, продажу почтовых марок и услуги экспресс-рассылки.

Для магазинов упаковки для почты, которые предоставляют услуги упаковки и продают марки, поздравительные открытки и упаковку, а также могут предоставлять услуги для UPS, Federal Express и других служб экспресс-почты, используется MCC 7399'),
        (9405, 'Внутригосударственные закупки - только государственные', 'Определяет транзакции между правительственными учреждениями, департаментами или агентствами, которые участвуют в программе внутригосударственных трансфертов (IGOTS).'),
        (9406, 'Государственные лотереи (кроме США)', 'Государственные лотереи, которые зарегистрированы в международной платежной системе для продажи лотерейных билетов. MCC используется для точек, расположенных за пределами США.'),
        (9411, 'Платежи по государственному займу', null),
        (9700, 'Automated Referral Service (только VISA)', 'Автоматизированная справочная служба - это сервис VisaNet, который позволяет авторизовать участника VIP. Ответ «реферальной» системы, чтобы набрать единственный бесплатный номер телефона, чтобы получить немедленный положительный или отрицательный ответ авторизации от эмитента, его уполномочивающего участника или резервного центра.'),
        (9701, 'Служба проверки учетных данных Visa (только VISA)', 'Использование ограничено процессом аутентификации для обеспечения транзакций банковских карт через Интернет и другие сети в режиме онлайн.'),
        (9702, 'Аварийные службы GCAS (только VISA)', 'Этот MCC предназначен для использования Visa только для того, чтобы члены и обработчики могли идентифицировать экстренные транзакции из Глобальных служб поддержки клиентов (GCAS).'),
        (9751, 'Супермаркеты (Великобритания)', 'Используется системой TSYS только для универсамов Великобритании, которые внесены в список Electronic Hot File.'),
        (9752, 'Автозаправочные станции (Великобритания)', 'Используется системой TSYS только для автозаправочных станций Великобритании, которые внесены в список Electronic Hot File.'),
        (9754, 'Лошадиные / собачьи бега (только США)', 'MCC код не используется, вместо него используется MCC 7802'),
        (9950, 'Покупки внутри компании', 'Точки, классифицируемые платежной системой Visa в рамках этого MCC обрабатывают транзакции по закупкам, представляющим внутренние переводы товаров и услуг между подразделениями. В остальных МПС данный код не используется и зарезервирован для будущего использования.'),
        (9999, 'Категория неизвестна', 'Возможно, код используется для операций с неопределенной категорией, т.к. в некоторых банках это код считается неклассифицируемой операцией.
В МКБ этот код присваивается операциям по оплате услуг в «МКБ Онлайн».

В стандартах СБП этот код присвоен категории "Услуги самозанятых граждан": Оплата услуг физических лиц и индивидуальных предпринимателей, являющихся налогоплательщиками налога на профессиональный доход в соответствии с законодательством Российской Федерации, оказывающих без привлечения наемных работников услуги физическому лицу для личных, домашних и (или) иных подобных нужд');